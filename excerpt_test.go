package main

import "testing"

func TestStripTagsUnescapesHTML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "numeric entities for quotes",
			in:   `<p>so here is the &#34;real&#34; release</p>`,
			want: `so here is the "real" release`,
		},
		{
			name: "angle bracket entities",
			in:   `<p>use &lt;div&gt; tags</p>`,
			want: `use <div> tags`,
		},
		{
			name: "ampersand entity",
			in:   `<p>rock &amp; roll</p>`,
			want: `rock & roll`,
		},
		{
			name: "no entities just tags",
			in:   `<p>plain text <strong>bold</strong></p>`,
			want: `plain text bold`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripTags(tt.in)
			if got != tt.want {
				t.Errorf("stripTags() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTruncateExcerpt(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		limit int
		want  string
	}{
		{
			name:  "short text no truncation",
			text:  "hello world",
			limit: 100,
			want:  "hello world",
		},
		{
			name:  "truncate at word boundary",
			text:  "hello world foo bar",
			limit: 14,
			want:  "hello world…",
		},
		{
			name:  "no space hard cut",
			text:  "supercalifragilistic",
			limit: 10,
			want:  "supercalif…",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateExcerpt(tt.text, tt.limit)
			if got != tt.want {
				t.Errorf("truncateExcerpt() = %q, want %q", got, tt.want)
			}
		})
	}
}
