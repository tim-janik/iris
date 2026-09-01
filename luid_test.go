package main

import (
	"strings"
	"testing"
)

func TestComputeLUIDIsPaddedToSevenChars(t *testing.T) {
	hrefs := []string{
		"",
		"/",
		"/index.html",
		"/blog/2024/07/my-post.html",
		"/a/b/c/d/e/f/g/h.html",
		"https://example.com/some/page.html?query=1#frag",
	}
	for _, href := range hrefs {
		got := computeLUID(href)
		if len(got) != 7 {
			t.Errorf("computeLUID(%q) = %q, want 7 characters", href, got)
		}
		for _, r := range got {
			if !strings.ContainsRune(base62, r) {
				t.Errorf("computeLUID(%q) = %q contains invalid base62 character %q", href, got, r)
			}
		}
	}
}

func TestComputeLUIDIsStable(t *testing.T) {
	href := "/blog/2024/07/my-post.html"
	first := computeLUID(href)
	for i := 0; i < 5; i++ {
		if again := computeLUID(href); again != first {
			t.Fatalf("computeLUID(%q) is not stable: %q vs %q", href, first, again)
		}
	}
	if first == computeLUID("/blog/2024/07/other-post.html") {
		t.Fatalf("distinct hrefs must produce distinct LUIDs")
	}
}
