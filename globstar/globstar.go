// This Source Code Form is licensed MPL-2.0: http://mozilla.org/MPL/2.0
//
// Package globstar provides glob pattern matching with ** (recursive) support.
//
// Patterns use forward-slash separated segments. Each segment is matched
// against the corresponding path segment using filepath.Match semantics
// (*, ?, [abc]). The special segment "**" matches zero or more path segments,
// enabling recursive directory matching.
//
// The package follows the regexp-style API: compile once (Compile), match many
// times (Pattern.Match). Convenience functions (Match, MatchAny) are provided
// for one-off use.
//
// Example:
//
//	p, _ := globstar.Compile("20*/**/*.md")
//	p.Match("2025/posts/hello.md") // true
//
//	globstar.Match("content/**/*.md", "content/foo.md") // true
//	globstar.MatchAny([]string{"*.md", "*.adoc"}, "readme.md") // true
package globstar

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Pattern is a compiled glob pattern that can be efficiently matched against
// multiple paths. Compile a pattern once, then reuse it.
type Pattern struct {
	parts []string // pre-split pattern segments
}

// Compile parses a glob pattern string into a Pattern. Returns an error for
// malformed patterns (e.g. unmatched brackets in a segment).
//
// This is the analogue of regexp.Compile. Use MustCompile when the pattern
// is known to be valid at compile time.
func Compile(pattern string) (*Pattern, error) {
	parts := strings.Split(pattern, "/")
	for _, p := range parts {
		if p == "**" {
			continue // ** is always valid
		}
		// Probe filepath.Match to catch malformed bracket expressions early.
		// We match against "" because we only care about the error, not the result.
		if _, err := filepath.Match(p, ""); err != nil {
			return nil, fmt.Errorf("globstar: invalid pattern %q: %w", pattern, err)
		}
	}
	return &Pattern{parts: parts}, nil
}

// MustCompile is like Compile but panics if the pattern is invalid.
func MustCompile(pattern string) *Pattern {
	p, err := Compile(pattern)
	if err != nil {
		panic(err)
	}
	return p
}

// Match reports whether path matches the compiled pattern.
func (p *Pattern) Match(path string) bool {
	segs := strings.Split(path, "/")
	return matchParts(p.parts, segs)
}

// matchParts recursively matches pattern parts against path segments.
// "**" matches zero or more path segments; all other segments use
// filepath.Match semantics (*, ?, [abc]).
func matchParts(parts, segs []string) bool {
	if len(parts) == 0 {
		return len(segs) == 0
	}
	// "**" matches zero or more segments
	if parts[0] == "**" {
		for i := 0; i <= len(segs); i++ {
			if matchParts(parts[1:], segs[i:]) {
				return true
			}
		}
		return false
	}
	// Non-"**" segment requires at least one remaining path segment
	if len(segs) == 0 {
		return false
	}
	ok, err := filepath.Match(parts[0], segs[0])
	return err == nil && ok && matchParts(parts[1:], segs[1:])
}

// Glob expands the pattern against a list of candidate paths, returning all
// matches in the order they appear in the input. Mirrors the POSIX glob(3)
// result collection semantics. Returns an empty slice (never nil) when no
// paths match.
func (p *Pattern) Glob(paths []string) []string {
	matches := make([]string, 0, len(paths))
	for _, path := range paths {
		if p.Match(path) {
			matches = append(matches, path)
		}
	}
	return matches
}

// String returns the original pattern string.
func (p *Pattern) String() string {
	return strings.Join(p.parts, "/")
}

// ---------------------------------------------------------------------------
// Convenience functions (no pre-compilation; for one-off use)
// ---------------------------------------------------------------------------

// Match matches a glob pattern against a path.
// Both pattern and path use forward slashes as separators.
//
// The special segment "**" matches zero or more path segments.
// All other segments use filepath.Match semantics (*, ?, [abc]).
//
// For repeated matching, use Compile() to pre-compile the pattern.
func Match(pattern, path string) bool {
	p, err := Compile(pattern)
	if err != nil {
		return false // malformed pattern → no match
	}
	return p.Match(path)
}

// MatchAny returns true if path matches any pattern in the list.
// Returns false for nil or empty pattern lists.
func MatchAny(patterns []string, path string) bool {
	for _, p := range patterns {
		if Match(p, path) {
			return true
		}
	}
	return false
}

// IsHidden returns true if any path segment starts with '.'.
func IsHidden(path string) bool {
	for _, seg := range strings.Split(path, "/") {
		if len(seg) > 0 && seg[0] == '.' {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Matcher — pre-compiled set of patterns (for include/exclude lists)
// ---------------------------------------------------------------------------

// Matcher is a pre-compiled set of glob patterns. Build once, match many
// paths against the set.
type Matcher struct {
	patterns []*Pattern
}

// NewMatcher compiles multiple patterns into a single Matcher.
// Returns the first error encountered; partial compilation is not performed.
func NewMatcher(patterns []string) (*Matcher, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	compiled := make([]*Pattern, 0, len(patterns))
	for _, p := range patterns {
		cp, err := Compile(p)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, cp)
	}
	return &Matcher{patterns: compiled}, nil
}

// Match reports whether path matches any of the compiled patterns.
// Returns false if the Matcher is nil.
func (m *Matcher) Match(path string) bool {
	if m == nil {
		return false
	}
	for _, p := range m.patterns {
		if p.Match(path) {
			return true
		}
	}
	return false
}

// Glob expands all compiled patterns against a list of candidate paths,
// returning the union of matches (deduplicated, preserving input order).
func (m *Matcher) Glob(paths []string) []string {
	if m == nil {
		return nil
	}
	seen := make(map[string]bool, len(paths))
	var matches []string
	for _, path := range paths {
		if seen[path] {
			continue
		}
		if m.Match(path) {
			seen[path] = true
			matches = append(matches, path)
		}
	}
	return matches
}

// ---------------------------------------------------------------------------
// Filter — include/exclude file filtering with pre-compiled matchers
// ---------------------------------------------------------------------------

// Filter holds pre-compiled include/exclude matchers for file filtering.
// Build with NewFilter for efficient repeated use (e.g. during directory walks).
type Filter struct {
	Include *Matcher // nil means no include filter (everything passes)
	Exclude *Matcher
}

// NewFilter compiles include/exclude pattern lists into a Filter.
// Returns the first error encountered during compilation.
func NewFilter(include, exclude []string) (*Filter, error) {
	var f Filter

	if len(include) > 0 {
		var err error
		f.Include, err = NewMatcher(include)
		if err != nil {
			return nil, fmt.Errorf("include pattern: %w", err)
		}
	}
	if len(exclude) > 0 {
		var err error
		f.Exclude, err = NewMatcher(exclude)
		if err != nil {
			return nil, fmt.Errorf("exclude pattern: %w", err)
		}
	}
	return &f, nil
}

// ShouldInclude determines whether a path should be included.
// Order of checks:
//  1. Hidden files (any segment starting with '.') are blocked unless
//     explicitly listed in Include patterns.
//  2. Include filter: if set, path must match at least one pattern.
//  3. Exclude filter: always applied last (highest priority).
func (f *Filter) ShouldInclude(path string) bool {
	// Hidden files: only pass if explicitly listed in include patterns
	if IsHidden(path) {
		if f.Include == nil {
			return false
		}
		if !f.Include.Match(path) {
			return false
		}
	}

	// Include filter: must match if set
	if f.Include != nil && !f.Include.Match(path) {
		return false
	}

	// Exclude filter: always applied last (highest priority)
	if f.Exclude != nil && f.Exclude.Match(path) {
		return false
	}

	return true
}
