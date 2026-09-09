package main

import "testing"

func TestValidateOutputCollisions(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  bool
	}{
		{name: "different sources collide", paths: []string{"foo.md", "foo.adoc"}, want: true},
		{name: "static and rendered collide", paths: []string{"foo.md", "foo.html"}, want: true},
		{name: "generated feed collides", paths: []string{"rss2.xml"}, want: true},
		{name: "embedded asset collides", paths: []string{"assets/highlight.js/highlight.min.js"}, want: true},
		{name: "file directory conflict", paths: []string{"docs", "docs/guide.md"}, want: true},
		{name: "generated index is suppressed by source", paths: []string{"2024/post.md", "2024/index.html"}, want: false},
		{name: "distinct outputs", paths: []string{"about.md", "2024/post.md", "assets/site.css"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateOutputCollisions(test.paths)
			if (err != nil) != test.want {
				t.Fatalf("validateOutputCollisions(%#v) = %v, want error %v", test.paths, err, test.want)
			}
		})
	}
}
