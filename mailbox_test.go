package main

import (
	"bytes"
	"encoding/base64"
	"mime/multipart"
	"mime/quotedprintable"
	"strings"
	"testing"
)

func TestRenderCommentHTMLEscapesName(t *testing.T) {
	c := Comment{
		ID:       "_20240101120000-Sender",
		FromName: `Eve <script>alert("hi")</script>`,
		Date:     "2024-01-01",
		Body:     "hello & <b>bye</b>",
	}
	out := renderCommentHTML(c)
	if strings.Contains(out, "<script>") {
		t.Errorf("renderCommentHTML() leaks unescaped name:\n%s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Errorf("renderCommentHTML() does not escape the sender name:\n%s", out)
	}
	if !strings.Contains(out, "hello &amp; &lt;b&gt;bye&lt;/b&gt;") {
		t.Errorf("renderCommentHTML() does not escape the body:\n%s", out)
	}
}

func TestCleanCommentBodyStripsLastSignature(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "earlier -- lines are preserved",
			in:   "step one\n-- done\nstep two\n-- \nJohn Doe",
			want: "step one\n-- done\nstep two",
		},
		{
			name: "signature at end",
			in:   "nice post!\n-- \nJohn Doe",
			want: "nice post!",
		},
		{
			name: "no signature",
			in:   "just a comment",
			want: "just a comment",
		},
		{
			name: "-- inside a line is kept",
			in:   "a -- b\nmore text",
			want: "a -- b\nmore text",
		},
		{
			name: "body only signature",
			in:   "-- \nJohn",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanCommentBody(tt.in); got != tt.want {
				t.Errorf("cleanCommentBody() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseMultipartBodyDecodesEncodings(t *testing.T) {
	tests := []struct {
		name     string
		encoding string
		payload  func(text string) ([]byte, string)
	}{
		{
			name:     "quoted-printable",
			encoding: "quoted-printable",
			payload: func(text string) ([]byte, string) {
				var buf bytes.Buffer
				w := quotedprintable.NewWriter(&buf)
				w.Write([]byte(text))
				w.Close()
				return buf.Bytes(), "quoted-printable"
			},
		},
		{
			name:     "base64",
			encoding: "base64",
			payload: func(text string) ([]byte, string) {
				return []byte(base64.StdEncoding.EncodeToString([]byte(text))), "base64"
			},
		},
		{
			name:     "plain",
			encoding: "",
			payload: func(text string) ([]byte, string) {
				return []byte(text), ""
			},
		},
	}

	const text = "first comment line\r\nsecond -- line"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, cte := tt.payload(text)
			var buf bytes.Buffer
			w := multipart.NewWriter(&buf)
			headers := map[string][]string{
				"Content-Type": {"text/plain; charset=utf-8"},
			}
			if cte != "" {
				headers["Content-Transfer-Encoding"] = []string{cte}
			}
			part, err := w.CreatePart(headers)
			if err != nil {
				t.Fatalf("CreatePart: %v", err)
			}
			part.Write(body)
			htmlPart, err := w.CreatePart(map[string][]string{
				"Content-Type": {"text/html"},
			})
			if err != nil {
				t.Fatalf("CreatePart: %v", err)
			}
			htmlPart.Write([]byte("<p>ignored</p>"))
			w.Close()

			got := parseMultipartBody(multipart.NewReader(&buf, w.Boundary()))
			want := strings.ReplaceAll(text, "\r\n", "\n")
			if got != want {
				t.Errorf("parseMultipartBody() = %q, want %q", got, want)
			}
		})
	}
}

func TestParseMultipartBodyRejectsTruncatedPart(t *testing.T) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreatePart(map[string][]string{
		"Content-Type": {"text/plain"},
	})
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	part.Write([]byte("a partial comment line"))
	w.Close()

	// Simulate a cut-off message by dropping the closing boundary.
	raw := buf.Bytes()
	cut := bytes.LastIndex(raw, []byte("\r\n--"))
	if cut < 0 {
		t.Fatalf("no closing boundary found in %q", raw)
	}

	got := parseMultipartBody(multipart.NewReader(bytes.NewReader(raw[:cut]), w.Boundary()))
	if got != "" {
		t.Errorf("parseMultipartBody() = %q for a truncated part, want empty", got)
	}
}
