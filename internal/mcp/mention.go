package mcp

import (
	"regexp"
	"strings"
)

// mentionPattern matches @ToolName tokens in user input.
// The leading group requires a word boundary (start of string or non-word char)
// so that addresses like "me@example.com" are not treated as mentions.
// Go's regexp has no lookbehind, so we capture the prefix in group 1 and the
// tool name in group 2; offsets are adjusted accordingly in ParseMentions.
var mentionPattern = regexp.MustCompile(`(^|[^A-Za-z0-9_.])@([A-Za-z][A-Za-z0-9_\-]*)`)

// Mention is one parsed @ToolName token.
type Mention struct {
	// Raw is the full matched string including the @ prefix.
	Raw string
	// Name is the tool name without the @ prefix.
	Name string
	// Start and End are byte offsets in the original text.
	Start, End int
}

// ParseMentions extracts all @ToolName mentions from text.
func ParseMentions(text string) []Mention {
	matches := mentionPattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]Mention, 0, len(matches))
	for _, m := range matches {
		// m[0]:m[1] = full match (includes optional prefix char)
		// m[2]:m[3] = prefix group (^|non-word)
		// m[4]:m[5] = tool name
		atStart := m[0] + (m[3] - m[2]) // skip the prefix char
		out = append(out, Mention{
			Raw:   text[atStart:m[1]],
			Name:  text[m[4]:m[5]],
			Start: atStart,
			End:   m[1],
		})
	}
	return out
}

// StripMentions removes all @ToolName tokens from text and returns the cleaned
// string along with the extracted tool names in order of appearance.
func StripMentions(text string) (cleaned string, names []string) {
	mentions := ParseMentions(text)
	if len(mentions) == 0 {
		return text, nil
	}

	names = make([]string, 0, len(mentions))
	var sb strings.Builder
	last := 0
	for _, m := range mentions {
		sb.WriteString(text[last:m.Start])
		names = append(names, m.Name)
		last = m.End
	}
	sb.WriteString(text[last:])
	cleaned = strings.TrimSpace(sb.String())
	return cleaned, names
}

// ResolveMentions maps each mentioned tool name to the first matching ToolDef
// in the provided slice. Unresolved names are returned in the second slice.
func ResolveMentions(names []string, available []ToolDef) (resolved []ToolDef, unresolved []string) {
	index := make(map[string]ToolDef, len(available))
	for _, t := range available {
		index[t.Name] = t
		// Also index by lowercase to allow case-insensitive @mentions.
		index[strings.ToLower(t.Name)] = t
	}

	for _, name := range names {
		if def, ok := index[name]; ok {
			resolved = append(resolved, def)
		} else if def, ok := index[strings.ToLower(name)]; ok {
			resolved = append(resolved, def)
		} else {
			unresolved = append(unresolved, name)
		}
	}
	return resolved, unresolved
}
