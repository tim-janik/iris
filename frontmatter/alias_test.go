package frontmatter

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestBoundedAliases(t *testing.T) {
	if os.Getenv("IRIS_TEST_YAML_ALIASES") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBoundedAliases$")
		cmd.Env = append(os.Environ(), "IRIS_TEST_YAML_ALIASES=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("alias subprocess: %s, %v", out, err)
		}
		return
	}
	expansion := "x0: &x0 [a, b]\n"
	for i := 1; i < 20; i++ {
		expansion += fmt.Sprintf("x%d: &x%d [*x%d, *x%d]\n", i, i, i-1, i-1)
	}
	large := "x: &x " + strings.Repeat("a", 100000) + "\ny: [" + strings.Repeat("*x, ", 20) + "]\n"
	for _, header := range []string{"x: &x [*x]\n", "x: &x {y: *x}\n", expansion, large, "x: " + strings.Repeat("[", 110) + "a" + strings.Repeat("]", 110)} {
		fm, body := Parse([]byte("---\ntitle: ignored\n"+header+"\n---\nbody"), "fallback.md")
		if fm.Title != "fallback" || len(fm.Raw) != 0 || body != "body" {
			t.Fatalf("accepted unsafe header: %+v, %q", fm, body)
		}
	}
	fm, body := Parse([]byte("---\ntitle: &title Shared\ncopy: *title\nlist: &list [one, two]\nother: *list\n---\nbody"))
	if fm.Title != "Shared" || fm.Raw["copy"] != "Shared" || fm.Raw["other"] != `["one","two"]` || body != "body" {
		t.Fatalf("shared aliases: %+v, %q", fm, body)
	}
}
