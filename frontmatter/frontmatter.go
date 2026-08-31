// Package frontmatter parses the YAML frontmatter used by Iris source files.
package frontmatter

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v4"
)

// Frontmatter holds the fields Iris understands plus arbitrary fields.
// Raw deliberately contains strings: YAML's implicit typing would otherwise
// turn dates, numbers, and booleans into Go values and lose their source
// semantics (for example, a deadline is a day, not a timestamp).
type Frontmatter struct {
	Title       string
	Description string
	Keywords    []string
	Published   string
	Authors     []string
	Raw         map[string]string

	// TitleSynthesized is true when Title came from the source filename rather
	// than frontmatter.  A converter can use this to provide a title explicitly
	// when the document has no H1 (pandoc otherwise rejects such documents).
	TitleSynthesized bool
}

// Parse splits a markdown document into frontmatter and body. sourceName is
// the source filename (or path) used to synthesize a title when the document
// has no non-empty lower-case title field. Frontmatter is delimited by --- at
// the start and --- or ... on its own line. A document without a complete
// frontmatter block is returned unchanged, with a synthesized title when a
// source name was supplied.
func Parse(content []byte, sourceName ...string) (*Frontmatter, string) {
	name := ""
	if len(sourceName) > 0 {
		name = sourceName[0]
	}
	text := string(content)
	fm := &Frontmatter{Raw: make(map[string]string)}

	blockStart, blockEnd, bodyStart, ok := frontmatterBlock(text)
	if ok {
		var doc yaml.Node
		if err := yaml.Unmarshal([]byte(text[blockStart:blockEnd]), &doc); err == nil && len(doc.Content) > 0 {
			root := doc.Content[0]
			if root.Kind == yaml.MappingNode {
				readMapping(fm, root)
			}
		}
		text = text[bodyStart:]
	}

	if strings.TrimSpace(fm.Title) == "" {
		fm.Title = titleFromSource(name)
		fm.TitleSynthesized = fm.Title != ""
	}
	return fm, text
}

// frontmatterBlock returns byte offsets for the YAML block and body. Looking
// at complete lines avoids mistaking --- inside a YAML scalar for a delimiter
// and also accepts a final delimiter at EOF.
func frontmatterBlock(text string) (blockStart, blockEnd, bodyStart int, ok bool) {
	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		return 0, 0, 0, false
	}
	openingEnd := strings.IndexByte(text, '\n') + 1
	if openingEnd == 0 {
		return 0, 0, 0, false
	}
	for pos := openingEnd; pos < len(text); {
		lineEnd := strings.IndexByte(text[pos:], '\n')
		end := len(text)
		next := len(text)
		if lineEnd >= 0 {
			end = pos + lineEnd
			next = end + 1
		}
		line := strings.TrimSuffix(text[pos:end], "\r")
		if line == "---" || line == "..." {
			return openingEnd, pos, next, true
		}
		if lineEnd < 0 {
			break
		}
		pos = next
	}
	return 0, 0, 0, false
}

func readMapping(fm *Frontmatter, root *yaml.Node) {
	for i := 0; i+1 < len(root.Content); i += 2 {
		keyNode, valueNode := root.Content[i], root.Content[i+1]
		key := keyNode.Value
		value := nodeString(valueNode)

		// Known keys are intentionally lower-case only. The one historical
		// exception is the capitalized singular Author alias.
		switch key {
		case "title":
			fm.Title = value
		case "description":
			fm.Description = value
		case "published":
			fm.Published = value
		case "keywords":
			fm.Keywords = nodeStrings(valueNode)
		case "authors":
			fm.Authors = nodeStrings(valueNode)
		case "Author":
			if strings.TrimSpace(value) != "" {
				fm.Authors = append(fm.Authors, value)
			}
		default:
			if key != "" {
				fm.Raw[key] = value
			}
		}
	}
	fm.Keywords = splitCommaValues(fm.Keywords)
	fm.Authors = splitCommaValues(fm.Authors)
}

func titleFromSource(sourceName string) string {
	if sourceName == "" {
		return ""
	}
	base := filepath.Base(filepath.ToSlash(sourceName))
	if ext := filepath.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}

// nodeStrings supports both YAML sequences and the comma-separated scalar
// spelling accepted by the original Iris parser.
func nodeStrings(node *yaml.Node) []string {
	if node.Kind == yaml.SequenceNode {
		out := make([]string, 0, len(node.Content))
		for _, child := range node.Content {
			out = append(out, nodeString(child))
		}
		return out
	}
	if node.Kind == yaml.ScalarNode {
		return []string{node.Value}
	}
	return nil
}

func splitCommaValues(values []string) []string {
	var out []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

// nodeString intentionally uses Node.Value for scalars. It is the lexical
// YAML value and therefore does not apply YAML's date/bool/number coercions.
// Composite values are compact JSON strings so Raw remains map[string]string.
func nodeString(node *yaml.Node) string {
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		return nodeString(node.Alias)
	}
	if node.Kind == yaml.ScalarNode {
		return node.Value
	}
	value := nodeJSONValue(node)
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func nodeJSONValue(node *yaml.Node) any {
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		return nodeJSONValue(node.Alias)
	}
	switch node.Kind {
	case yaml.SequenceNode:
		out := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			out = append(out, nodeJSONValue(child))
		}
		return out
	case yaml.MappingNode:
		out := make(map[string]any, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			out[node.Content[i].Value] = nodeJSONValue(node.Content[i+1])
		}
		return out
	case yaml.DocumentNode:
		if len(node.Content) > 0 {
			return nodeJSONValue(node.Content[0])
		}
	}
	return node.Value
}
