// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"log"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Comment mailbox: parse .eml files and generate comment HTML
// ---------------------------------------------------------------------------

// MailboxConfig holds configuration for the comment mailbox.
type MailboxConfig struct {
	CommentsDir   string // directory containing .eml files
	CommentsEmail string // email template, e.g. "newcomment+%s@testbit.eu"
}

// Comment represents a single parsed comment from an .eml file.
type Comment struct {
	ID        string // unique identifier for anchor links
	FromName  string // sanitized sender name
	Date      string // YYYY-MM-DD
	Timestamp string // YYYYMMDDHHmmSS for the anchor ID
	Body      string // cleaned comment body text
}

// wordDecoder decodes MIME-encoded-words in header values (RFC 2047).
var wordDecoder = mime.WordDecoder{}

// LoadComments scans the comments directory for .eml files, parses them,
// and returns comments grouped by LUID.
// Returns a map from LUID to a sorted slice of pre-rendered comment HTML strings.
func LoadComments(cfg MailboxConfig, pages []*InputPage) map[string][]string {
	if cfg.CommentsDir == "" {
		return nil
	}

	// Collect .eml files
	emlFiles, err := filepath.Glob(filepath.Join(cfg.CommentsDir, "*.eml"))
	if err != nil || len(emlFiles) == 0 {
		return nil
	}

	// Build LUID-to-recipient map (case-insensitive)
	luidToRecipient := make(map[string]string)
	recipientToLuid := make(map[string]string)
	for _, pg := range pages {
		if cfg.CommentsEmail != "" {
			luid := pg.PageLUID()
			email := fmt.Sprintf(cfg.CommentsEmail, luid)
			luidToRecipient[strings.ToLower(email)] = luid
			recipientToLuid[luid] = email
		}
	}

	// Parse each .eml file
	commentsByLuid := make(map[string][]Comment)

	for _, emlFile := range emlFiles {
		data, err := os.ReadFile(emlFile)
		if err != nil {
			log.Printf("  mailbox: read %s: %v", emlFile, err)
			continue
		}

		msg, err := mail.ReadMessage(bytes.NewReader(data))
		if err != nil {
			log.Printf("  mailbox: parse %s: %v", emlFile, err)
			continue
		}

		// Match recipient (To or CC) to LUID
		luid := matchRecipient(msg, luidToRecipient)
		if luid == "" {
			continue
		}

		comment := processComment(msg)
		if comment == nil {
			continue
		}

		commentsByLuid[luid] = append(commentsByLuid[luid], *comment)
	}

	// Sort comments by timestamp (ascending) and render HTML
	result := make(map[string][]string)
	for luid, comments := range commentsByLuid {
		sort.Slice(comments, func(i, j int) bool {
			return comments[i].Timestamp < comments[j].Timestamp
		})

		htmls := make([]string, 0, len(comments))
		for _, c := range comments {
			htmls = append(htmls, renderCommentHTML(c))
		}
		result[luid] = htmls
	}

	return result
}

// decodeHeader decodes MIME-encoded-words from a header value (RFC 2047).
func decodeHeader(raw string) string {
	decoded, err := wordDecoder.DecodeHeader(raw)
	if err != nil {
		return raw
	}
	return decoded
}

// parseMultipartBody extracts the text/plain body from a multipart message.
func parseMultipartBody(r *multipart.Reader) string {
	for {
		part, err := r.NextPart()
		if err != nil {
			break
		}

		if strings.HasPrefix(part.Header.Get("Content-Type"), "text/plain") {
			var buf bytes.Buffer
			buf.ReadFrom(part)
			body := buf.String()
			body = strings.ReplaceAll(body, "\r\n", "\n")
			body = strings.ReplaceAll(body, "\r", "\n")
			return body
		}
	}
	return ""
}

// decodePart decodes a single body part based on its Content-Transfer-Encoding.
func decodePart(data []byte, encoding string) string {
	encoding = strings.ToLower(encoding)

	switch encoding {
	case "quoted-printable":
		decoder := quotedprintable.NewReader(bytes.NewReader(data))
		var buf bytes.Buffer
		buf.ReadFrom(decoder)
		data = buf.Bytes()
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(data)))
		if err == nil {
			data = decoded
		}
		// "7bit", "8bit" or empty: no decoding needed
	}

	result := strings.ReplaceAll(string(data), "\r\n", "\n")
	result = strings.ReplaceAll(result, "\r", "\n")
	return result
}

// matchRecipient matches the To/CC header to a known LUID using net/mail.
func matchRecipient(msg *mail.Message, luidToRecipient map[string]string) string {
	for _, headerName := range []string{"To", "Cc"} {
		addresses, err := mail.ParseAddressList(msg.Header.Get(headerName))
		if err != nil {
			continue
		}
		for _, addr := range addresses {
			addrLower := strings.ToLower(addr.Address)
			if luid, ok := luidToRecipient[addrLower]; ok {
				return luid
			}
		}
	}

	// Fallback: try partial local-part matching for newcomment+<luid>@ patterns
	for _, headerName := range []string{"To", "Cc"} {
		header := strings.ToLower(msg.Header.Get(headerName))
		// Extract local parts (before @) that match newcomment+<luid>
		parts := strings.Split(header, ",")
		for _, part := range parts {
			atIdx := strings.Index(part, "@")
			if atIdx <= 0 {
				continue
			}
			local := strings.TrimSpace(part[:atIdx])
			plusIdx := strings.Index(local, "newcomment+")
			if plusIdx < 0 {
				continue
			}
			luidPart := local[plusIdx+len("newcomment+"):]
			for email, luid := range luidToRecipient {
				if strings.Contains(email, "+"+luidPart+"@") {
					return luid
				}
			}
		}
	}

	return ""
}

// processComment extracts and cleans the comment data from an email message.
func processComment(msg *mail.Message) *Comment {
	subject := decodeHeader(msg.Header.Get("Subject"))
	fromRaw := decodeHeader(msg.Header.Get("From"))
	dateRaw := decodeHeader(msg.Header.Get("Date"))

	if subject == "" || fromRaw == "" || dateRaw == "" {
		return nil
	}

	fromName := sanitizeSender(fromRaw)
	date, timestamp := parseEmailDate(dateRaw)
	if date == "" {
		return nil
	}

	// Read body into buffer (msg.Body is io.Reader)
	var bodyBuf bytes.Buffer
	bodyBuf.ReadFrom(msg.Body)
	bodyData := bodyBuf.Bytes()

	// Extract body - handle multipart
	ct := msg.Header.Get("Content-Type")
	var body string
	if strings.HasPrefix(ct, "multipart/") {
		_, params, err := mime.ParseMediaType(ct)
		if err == nil {
			boundary := params["boundary"]
			if boundary != "" {
				r := multipart.NewReader(bytes.NewReader(bodyData), boundary)
				body = parseMultipartBody(r)
			}
		}
	} else {
		transferEncoding := msg.Header.Get("Content-Transfer-Encoding")
		body = decodePart(bodyData, transferEncoding)
	}

	body = cleanCommentBody(body)
	if body == "" {
		return nil
	}

	// Sanitize the ID: remove special characters that aren't valid in HTML IDs
	safeName := strings.ReplaceAll(fromName, " ", "_")
	safeName = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return -1
	}, safeName)
	id := fmt.Sprintf("_%s-%s", timestamp, safeName)

	return &Comment{
		ID:        id,
		FromName:  fromName,
		Date:      date,
		Timestamp: timestamp,
		Body:      body,
	}
}

// sanitizeSender strips email address from "Name <email>" and replaces @ with @email.
func sanitizeSender(from string) string {
	addr, err := mail.ParseAddress(from)
	if err != nil {
		// Fallback: basic cleaning
		from = strings.ReplaceAll(from, "<", " ")
		from = strings.ReplaceAll(from, ">", " ")
		atIdx := strings.Index(from, "@")
		if atIdx >= 0 {
			from = from[:atIdx] + "@email"
		}
		return strings.TrimSpace(from)
	}

	name := addr.Name
	if name == "" {
		name = addr.Address
	}
	// Replace bare @ with @email to prevent email harvesting
	name = strings.ReplaceAll(name, "@", "@email")
	name = strings.TrimSpace(name)
	name = strings.Trim(name, `"`)
	name = strings.TrimSpace(name)
	name = strings.Join(strings.Fields(name), " ")
	return name
}

// parseEmailDate extracts YYYY-MM-DD and YYYYMMDDHHmmSS from an email Date header.
// Timestamps are always in UTC.
func parseEmailDate(dateStr string) (string, string) {
	t, err := mail.ParseDate(dateStr)
	if err != nil {
		return "", ""
	}
	t = t.UTC()
	return t.Format(dateLayout), t.Format("20060102150405")
}

// cleanCommentBody strips email signatures, "Add comment to /path:" prefix,
// and collapses blank lines.
func cleanCommentBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")

	// Strip "Add comment to /somepath:" prefix (first line)
	if strings.HasPrefix(body, "Add comment to /") {
		firstNewline := strings.Index(body, "\n")
		if firstNewline > 0 {
			body = body[firstNewline+1:]
		}
	}

	// Strip email signature: lines starting with "-- " at the end
	if idx := strings.Index(body, "-- "); idx >= 0 {
		// Only strip if "-- " is at the start of a line
		if idx == 0 || body[idx-1] == '\n' {
			body = body[:idx]
		}
	}

	// Collapse multiple blank lines into double newlines
	for strings.Contains(body, "\n\n\n") {
		body = strings.ReplaceAll(body, "\n\n\n", "\n\n")
	}

	body = strings.TrimSpace(body)
	return body
}

// renderCommentHTML generates the HTML for a single comment.
func renderCommentHTML(c Comment) string {
	escapedBody := html.EscapeString(c.Body)

	return fmt.Sprintf(`<div class="sect1">
  <h3 id="%s">
    <span class="commentdate">%s</span>
    <span class="commenttile">%s </span>
  </h3>
  <div class="sectionbody">
    <div class="commentblock">
      <div class="content commentbody">
        <pre class="highlight"><code class="language-markdown" data-lang="markdown">%s</code></pre>
      </div>
    </div>
  </div>
</div>`, c.ID, c.Date, c.FromName, escapedBody)
}
