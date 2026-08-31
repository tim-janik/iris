// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
package globstar

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Compile / MustCompile
// ---------------------------------------------------------------------------

func TestCompile(t *testing.T) {
	// Valid patterns
	for _, pat := range []string{"*.md", "**", "20*/**", "a/**/b/**/c.txt", "", "exact", "[ab].txt"} {
		p, err := Compile(pat)
		if err != nil {
			t.Errorf("Compile(%q) unexpected error: %v", pat, err)
		}
		if p == nil {
			t.Errorf("Compile(%q) returned nil pattern", pat)
		}
	}

	// Invalid pattern: unmatched bracket
	_, err := Compile("[invalid")
	if err == nil {
		t.Error("Compile(\"[invalid\") should return error")
	}
}

func TestPatternString(t *testing.T) {
	p, _ := Compile("a/b/**/*.md")
	if got := p.String(); got != "a/b/**/*.md" {
		t.Errorf("Pattern.String() = %q, want %q", got, "a/b/**/*.md")
	}
}

// ---------------------------------------------------------------------------
// Pattern.Match
// ---------------------------------------------------------------------------

func TestPatternMatch(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		// ── Simple *.ext patterns (single segment) ─────────────────────
		{"*.md", "README.md", true},
		{"*.md", "2025/post.md", false},
		{"*.adoc", "notes.adoc", true},
		{"*.adoc", "notes.md", false},

		// ── ** recursive patterns ──────────────────────────────────────
		{"20*/**", "2025/foo.txt", true},
		{"20*/**", "2018/a/b.png", true},
		{"20*/**", "2025", true}, // ** matches zero segments
		{"**/*.md", "a/b/c.md", true},
		{"**/*.md", "c.md", true}, // ** matches zero segments
		{"**/*.md", "a/b/c/d/e.md", true},
		{"**", "anything/at/all", true},
		{"**", "", true}, // ** matches empty path (zero segments)

		// ── Exact match (no wildcards) ─────────────────────────────────
		{".htaccess", ".htaccess", true},
		{".htaccess", "sub/.htaccess", false},
		{"BACKLOG.md", "BACKLOG.md", true},
		{"BACKLOG.md", "old/BACKLOG.md", false},

		// ── Underscore prefix ──────────────────────────────────────────
		{"_*", "_templates", true},
		{"_*", "_config.toml", true},
		{"_*", "foo/_bar", false},

		// ── Question mark ──────────────────────────────────────────────
		{"?.txt", "a.txt", true},
		{"?.txt", "ab.txt", false},

		// ── Character class ────────────────────────────────────────────
		{"[ab].txt", "a.txt", true},
		{"[ab].txt", "c.txt", false},

		// ── Mixed patterns ─────────────────────────────────────────────
		{"content/**/*.md", "content/post.md", true},
		{"content/**/*.md", "content/2025/post.md", true},
		{"content/**/*.md", "content/a/b/c/post.md", true},
		{"content/**/*.md", "drafts/post.md", false},

		// ── Multiple ** in one pattern ─────────────────────────────────
		{"a/**/b/**/c.txt", "a/b/c.txt", true},
		{"a/**/b/**/c.txt", "a/x/b/y/c.txt", true},
		{"a/**/b/**/c.txt", "a/x/y/z/b/m/n/c.txt", true},

		// ── ** at start, middle, end ───────────────────────────────────
		{"**/README.md", "README.md", true},
		{"**/README.md", "docs/README.md", true},
		{"**/README.md", "a/b/c/README.md", true},
		{"src/**/test", "src/test", true},
		{"src/**/test", "src/unit/test", true},
		{"src/**/test", "src/a/b/c/test", true},

		// ── Empty edge cases ───────────────────────────────────────────
		{"", "", true},
		{"", "a", false},
		{"a", "", false},

		// ── Path segment count mismatch ────────────────────────────────
		{"a/b/c", "a/b", false},
		{"a/b", "a/b/c", false},
		{"a/b/c", "a/b/c", true},
	}

	for _, tt := range tests {
		p, err := Compile(tt.pattern)
		if err != nil {
			t.Fatalf("Compile(%q): %v", tt.pattern, err)
		}
		got := p.Match(tt.path)
		if got != tt.want {
			t.Errorf("Pattern.Match(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Pattern.Glob
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Convenience: MatchAny
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// IsHidden
// ---------------------------------------------------------------------------

func TestIsHidden(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{".git/config", true},
		{"foo/.hidden", true},
		{"a/b/.dot", true},
		{"normal/file.txt", false},
		{"2025/post.md", false},
		{".", true},
		{"..", true},
		{".env", true},
		{"src/.gitignore", true},
	}

	for _, tt := range tests {
		got := IsHidden(tt.path)
		if got != tt.want {
			t.Errorf("IsHidden(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Matcher (pre-compiled pattern set)
// ---------------------------------------------------------------------------

func TestNewMatcher(t *testing.T) {
	m, err := NewMatcher([]string{"*.md", "20*/**"})
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}
	if m == nil {
		t.Fatal("NewMatcher returned nil")
	}

	// Empty list → nil matcher
	m2, err := NewMatcher(nil)
	if err != nil {
		t.Fatalf("NewMatcher(nil): %v", err)
	}
	if m2 != nil {
		t.Error("NewMatcher(nil) should return nil")
	}

	// Invalid pattern → error
	_, err = NewMatcher([]string{"*.md", "[bad"})
	if err == nil {
		t.Error("NewMatcher with invalid pattern should return error")
	}
}

func TestMatcherMatch(t *testing.T) {
	m, _ := NewMatcher([]string{"*.md", "20*/**"})

	tests := []struct {
		path string
		want bool
	}{
		{"readme.md", true},
		{"2025/post.txt", true},
		{"style.css", false},
	}

	for _, tt := range tests {
		got := m.Match(tt.path)
		if got != tt.want {
			t.Errorf("Matcher.Match(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}

	// nil matcher → always false
	var nilM *Matcher
	if nilM.Match("anything") {
		t.Error("nil Matcher.Match should return false")
	}
}

// ---------------------------------------------------------------------------
// Filter (pre-compiled include/exclude)
// ---------------------------------------------------------------------------

func TestNewFilter(t *testing.T) {
	f, err := NewFilter([]string{"20*/**"}, []string{"_*"})
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}
	if f.Include == nil || f.Exclude == nil {
		t.Error("NewFilter should populate both Include and Exclude")
	}

	// Empty lists → nil matchers
	f2, err := NewFilter(nil, nil)
	if err != nil {
		t.Fatalf("NewFilter(nil, nil): %v", err)
	}
	if f2.Include != nil || f2.Exclude != nil {
		t.Error("NewFilter(nil, nil) should produce nil matchers")
	}

	// Invalid pattern → error
	_, err = NewFilter([]string{"[bad"}, nil)
	if err == nil {
		t.Error("NewFilter with invalid include should return error")
	}
	_, err = NewFilter(nil, []string{"[bad"})
	if err == nil {
		t.Error("NewFilter with invalid exclude should return error")
	}
}

func TestFilterShouldInclude(t *testing.T) {
	// Default filter: only Exclude = ["_*"]
	defaultFilter, _ := NewFilter(nil, []string{"_*"})

	tests := []struct {
		path   string
		filter *Filter
		want   bool
	}{
		// Default filter: everything except _* passes
		{"README.md", defaultFilter, true},
		{"_siteconfig.toml", defaultFilter, false},
		{"2025/post.md", defaultFilter, true},
		{"style.css", defaultFilter, true},

		// Hidden files blocked by default filter (no include patterns)
		{".git/config", defaultFilter, false},
		{".htaccess", defaultFilter, false},

		// Include glob: only match listed patterns
		{"2025/post.md", mustNewFilter(t, []string{"20*/**"}, []string{"_*"}), true},
		{"style.css", mustNewFilter(t, []string{"20*/**"}, []string{"_*"}), false},
		{".htaccess", mustNewFilter(t, []string{".htaccess"}, []string{"_*"}), true},

		// Exclude always wins even if include matches
		{"BACKLOG.md", mustNewFilter(t, []string{"**"}, []string{"BACKLOG.md"}), false},

		// No include patterns means everything passes (subject to exclude + hidden)
		{"anything.txt", mustNewFilter(t, nil, nil), true},
		{".hidden", mustNewFilter(t, nil, nil), false}, // hidden always blocked without explicit include
	}

	for _, tt := range tests {
		got := tt.filter.ShouldInclude(tt.path)
		if got != tt.want {
			t.Errorf("Filter.ShouldInclude(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestFilterHiddenWithExplicitInclude(t *testing.T) {
	f, _ := NewFilter([]string{".*"}, nil)
	if !f.ShouldInclude(".gitignore") {
		t.Error("hidden file should pass when explicitly included")
	}
}

func TestFilterExcludeOverridesInclude(t *testing.T) {
	f, _ := NewFilter([]string{"**"}, []string{"secret/*"})
	if f.ShouldInclude("secret/password.txt") {
		t.Error("exclude should override include")
	}
	if !f.ShouldInclude("public/hello.txt") {
		t.Error("non-excluded file should pass")
	}
}

// ---------------------------------------------------------------------------
// Performance: compiled vs uncompiled
// ---------------------------------------------------------------------------

func BenchmarkPatternMatch(b *testing.B) {
	p, _ := Compile("20*/**/*.md")
	for i := 0; i < b.N; i++ {
		p.Match("2025/posts/hello.md")
	}
}

func BenchmarkFilterShouldInclude(b *testing.B) {
	f, _ := NewFilter([]string{"20*/**", "*.md"}, []string{"_*", ".git/**"})
	for i := 0; i < b.N; i++ {
		f.ShouldInclude("2025/posts/hello.md")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustNewFilter(t *testing.T, include, exclude []string) *Filter {
	t.Helper()
	f, err := NewFilter(include, exclude)
	if err != nil {
		t.Fatalf("NewFilter(%v, %v): %v", include, exclude, err)
	}
	return f
}
