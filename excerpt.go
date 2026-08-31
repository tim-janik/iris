package main

import (
	"strings"
	"unicode"
)

// stripTags removes HTML tags from a string, returning plain text.
func stripTags(html string) string {
	var buf strings.Builder
	inTag := false
	for _, r := range html {
		if r == '<' {
			inTag = true
		} else if r == '>' {
			inTag = false
		} else if !inTag {
			// Normalize whitespace: collapse runs of whitespace into single spaces
			if r == '\n' || r == '\r' || r == '\t' {
				r = ' '
			}
			buf.WriteRune(r)
		}
	}
	// Collapse multiple spaces into one
	result := strings.Join(strings.Fields(buf.String()), " ")
	return strings.TrimSpace(result)
}

// truncateExcerpt truncates text to at most limit characters (runes), breaking
// at a word boundary. If truncation occurred it strips trailing non-letter
// characters and appends an ellipsis (…). If the text fits within the limit
// it is returned unchanged.
func truncateExcerpt(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	// Find the last space before the limit so we break at a word boundary
	cut := limit
	for cut > 0 && runes[cut] != ' ' {
		cut--
	}
	if cut == 0 {
		cut = limit // no space found, hard cut
	}
	// Strip trailing non-letter characters (punctuation, digits, etc.)
	for cut > 0 {
		if unicode.IsLetter(runes[cut-1]) {
			break
		}
		cut--
	}
	return string(runes[:cut]) + "…"
}
