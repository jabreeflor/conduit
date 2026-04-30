package design

import (
	"encoding/json"
	"fmt"
)

// TextualTheme is the JSON shape consumed by Textual/Rich.
// One file is emitted per mode.
type TextualTheme struct {
	Name       string            `json:"name"`
	Mode       string            `json:"mode"`
	Background string            `json:"background"`
	Foreground string            `json:"foreground"`
	Primary    string            `json:"primary"`
	Secondary  string            `json:"secondary"`
	Accent     string            `json:"accent"`
	Surface    string            `json:"surface"`
	Panel      string            `json:"panel"`
	Success    string            `json:"success"`
	Warning    string            `json:"warning"`
	Error      string            `json:"error"`
	Info       string            `json:"info"`
	Variables  map[string]string `json:"variables"`
}

// EmitTextual renders a Textual theme JSON for the given mode.
func EmitTextual(t *Tokens, mode string) (string, error) {
	tree, ok := t.Semantic[mode]
	if !ok {
		return "", fmt.Errorf("unknown mode %q", mode)
	}
	color, ok := tree["color"].(Tree)
	if !ok {
		return "", fmt.Errorf("mode %q: missing color subtree", mode)
	}

	theme := TextualTheme{
		Name:       "conduit-" + mode,
		Mode:       mode,
		Background: lookupColor(color, "surface", "canvas"),
		Foreground: lookupColor(color, "text", "body"),
		Primary:    lookupColor(color, "brand", "primary"),
		Secondary:  lookupColor(color, "accent", "cool"),
		Accent:     lookupColor(color, "accent", "warm-alt"),
		Surface:    lookupColor(color, "surface", "primary"),
		Panel:      lookupColor(color, "surface", "elevated"),
		Success:    lookupColor(color, "status", "success"),
		Warning:    lookupColor(color, "status", "warning"),
		Error:      lookupColor(color, "status", "error"),
		Info:       lookupColor(color, "status", "info"),
		Variables:  flattenColors(color),
	}
	out, err := json.MarshalIndent(theme, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out) + "\n", nil
}

func lookupColor(t Tree, path ...string) string {
	v, ok := lookup(t, path)
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func flattenColors(t Tree) map[string]string {
	out := map[string]string{}
	Walk(t, func(path []string, value string) {
		out[joinDots(path)] = value
	})
	return out
}

func joinDots(parts []string) string {
	s := ""
	for i, p := range parts {
		if i > 0 {
			s += "."
		}
		s += p
	}
	return s
}
