package editlink

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// Config holds settings for the editlink feature.
type Config struct {
	// Cmd is the command template used to open the source file in an editor.
	// If empty, edit links are disabled.
	//
	// Supported placeholders:
	//   %s — source file path (always substituted)
	//   %u — line number (only substituted if present in template)
	//
	// Example:
	//   gnome-terminal --geometry=132x50 -- $EDITOR +%u %s
	Cmd string
}

// OpenEditor opens the source file in the user's editor, jumping to the
// specified line. It launches the editor as a separate process (non-blocking).
func OpenEditor(cfg Config, srcPath string, lineNum int) error {
	if cfg.Cmd == "" {
		return fmt.Errorf("editlink command not configured")
	}

	// Split template at whitespace to get individual args
	args := strings.Fields(cfg.Cmd)

	// Substitute %s and %u in each arg
	for i, arg := range args {
		if strings.Contains(arg, "%s") {
			args[i] = strings.ReplaceAll(arg, "%s", srcPath)
		}
		if strings.Contains(arg, "%u") {
			args[i] = strings.ReplaceAll(arg, "%u", fmt.Sprintf("%d", lineNum))
		}
	}

	c := exec.Command(args[0], args[1:]...)
	log.Printf("[editlink] launching: %s", c.String())

	return c.Start()
}
