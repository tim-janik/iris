package frontmatter

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestParsePreservesUnknownValuesAndNormalizesFields(t *testing.T) {
	input := []byte("---\n" +
		"title: \"A task\"\n" +
		"description: Replace it\n" +
		"keywords: garden, repair\n" +
		"authors: [Ada, Bob]\n" +
		"Author: Eve\n" +
		"due: 2026-01-02\n" +
		"count: 42\n" +
		"enabled: true\n" +
		"quoted: '2026-01-02'\n" +
		"labels: [one, 2, false]\n" +
		"options: {color: green, retries: 2}\n" +
		"author: lowercase-is-unknown\n" +
		"---\n" +
		"Body\n")

	fm, body := Parse(input, "tasks/fix.md")
	if body != "Body\n" {
		t.Fatalf("body = %q", body)
	}
	if fm.Title != "A task" || fm.TitleSynthesized {
		t.Fatalf("title = %#v", fm)
	}
	if !reflect.DeepEqual(fm.Keywords, []string{"garden", "repair"}) {
		t.Fatalf("keywords = %#v", fm.Keywords)
	}
	if !reflect.DeepEqual(fm.Authors, []string{"Ada", "Bob", "Eve"}) {
		t.Fatalf("authors = %#v", fm.Authors)
	}
	for key, want := range map[string]string{
		"due":     "2026-01-02",
		"count":   "42",
		"enabled": "true",
		"quoted":  "2026-01-02",
		"author":  "lowercase-is-unknown",
	} {
		if got := fm.Raw[key]; got != want {
			t.Errorf("Raw[%q] = %q, want %q", key, got, want)
		}
	}
	var labels []string
	if err := json.Unmarshal([]byte(fm.Raw["labels"]), &labels); err != nil {
		t.Fatalf("labels JSON %q: %v", fm.Raw["labels"], err)
	}
	if !reflect.DeepEqual(labels, []string{"one", "2", "false"}) {
		t.Errorf("labels = %#v", labels)
	}
	var options map[string]string
	if err := json.Unmarshal([]byte(fm.Raw["options"]), &options); err != nil {
		t.Fatalf("options JSON %q: %v", fm.Raw["options"], err)
	}
	if options["retries"] != "2" || options["color"] != "green" {
		t.Errorf("options = %#v", options)
	}
}

func TestParseTerminatorsAndTitleSynthesis(t *testing.T) {
	for _, terminator := range []string{"---", "..."} {
		fm, body := Parse([]byte("---\nstatus: open\n"+terminator+"\n## Details\n"), "fix-fence.md")
		if fm.Title != "fix-fence" || !fm.TitleSynthesized {
			t.Errorf("%s title = %#v", terminator, fm)
		}
		if body != "## Details\n" {
			t.Errorf("%s body = %q", terminator, body)
		}
	}

	fm, body := Parse([]byte("---\ntitle: Broken\n## still body\n"), "missing.md")
	if fm.Title != "missing" || !fm.TitleSynthesized || body != "---\ntitle: Broken\n## still body\n" {
		t.Fatalf("incomplete frontmatter = %#v, body %q", fm, body)
	}

	fm, body = Parse([]byte("plain body\n"), "plain.md")
	if fm.Title != "plain" || !fm.TitleSynthesized || body != "plain body\n" {
		t.Fatalf("missing frontmatter = %#v, body %q", fm, body)
	}
}

func TestKnownKeysAreCanonical(t *testing.T) {
	fm, _ := Parse([]byte("---\nTitle: Wrong key\nAuthor: One\nauthor: Two\n---\n"), "example.md")
	if fm.Title != "example" || !fm.TitleSynthesized {
		t.Fatalf("title = %#v", fm)
	}
	if !reflect.DeepEqual(fm.Authors, []string{"One"}) {
		t.Fatalf("authors = %#v", fm.Authors)
	}
	if fm.Raw["Title"] != "Wrong key" || fm.Raw["author"] != "Two" {
		t.Fatalf("raw = %#v", fm.Raw)
	}
}

func TestH1Title(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{"# Title\n\nBody\n", "Title"},
		{"#\tTab Title\n", "Tab Title"},
		{"## Not H1\n\n# Real H1\n", "Real H1"},
		{"Intro\n\n## Section\n", ""},
		{"#  Spaced  \n", "Spaced"},
		{"", ""},
	} {
		if got := H1Title(tc.body); got != tc.want {
			t.Errorf("H1Title(%q) = %q, want %q", tc.body, got, tc.want)
		}
	}
}
