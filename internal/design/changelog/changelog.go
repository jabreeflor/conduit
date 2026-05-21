// Package changelog embeds CHANGELOG.md and surfaces the "what's new"
// section on first launch after an upgrade.
//
// PRD §17.5 calls for the changelog to be surfaced in-app on update. The
// TUI calls Latest() to get the most recent release section, and ShouldShow()
// to decide whether to render it based on the previously-seen version stored
// in ~/.conduit/state.json.
package changelog

import (
	_ "embed"
	"strings"
)

//go:embed embedded.md
var raw string

// Raw returns the embedded CHANGELOG.md verbatim.
func Raw() string { return raw }

// Section is one parsed top-level section of the changelog.
type Section struct {
	// Heading is the version label as it appears in the file, e.g. "v0.2.0"
	// or "Unreleased". Brackets and date suffix are stripped.
	Heading string
	// Body is the markdown content under the heading, without the leading
	// "## ..." line. Trailing whitespace is trimmed.
	Body string
}

// Parse splits the embedded changelog into ordered sections. The first
// returned section is the most recent release (or "Unreleased").
func Parse() []Section {
	return parse(raw)
}

func parse(src string) []Section {
	var out []Section
	var cur *Section
	var body strings.Builder

	flush := func() {
		if cur == nil {
			return
		}
		cur.Body = strings.TrimRight(body.String(), "\n")
		out = append(out, *cur)
		cur = nil
		body.Reset()
	}

	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(line, "## ") {
			flush()
			heading := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			heading = strings.TrimPrefix(heading, "[")
			if i := strings.Index(heading, "]"); i >= 0 {
				heading = heading[:i]
			}
			cur = &Section{Heading: heading}
			continue
		}
		if cur != nil {
			body.WriteString(line)
			body.WriteByte('\n')
		}
	}
	flush()
	return out
}

// Latest returns the first non-Unreleased section, or the Unreleased
// section if no release has been cut yet. Returns the zero value if the
// file is empty.
func Latest() Section {
	sections := Parse()
	for _, s := range sections {
		if !strings.EqualFold(s.Heading, "Unreleased") {
			return s
		}
	}
	if len(sections) > 0 {
		return sections[0]
	}
	return Section{}
}

// ShouldShow reports whether the in-app "what's new" panel should be
// rendered, given the version the user previously launched (lastSeen) and
// the version they're launching now (current). An empty lastSeen means a
// fresh install — the panel is shown so first-time users see the release
// notes.
func ShouldShow(lastSeen, current string) bool {
	if current == "" {
		return false
	}
	return strings.TrimSpace(lastSeen) != strings.TrimSpace(current)
}
