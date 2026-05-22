package keybindings

import (
	"fmt"
	"strings"
)

// validModifiers are the modifier tokens we accept in a chord. "mod" is the
// portable alias for "ctrl" — bubbletea reports both ctrl and cmd as ctrl on
// macOS terminals, so resolving mod -> ctrl is the practical choice for v1.
var validModifiers = map[string]string{
	"ctrl":    "ctrl",
	"control": "ctrl",
	"mod":     "ctrl",
	"cmd":     "ctrl", // tui surface treats cmd as ctrl; gui can override later
	"super":   "ctrl",
	"alt":     "alt",
	"option":  "alt",
	"opt":     "alt",
	"meta":    "alt",
	"shift":   "shift",
}

// modifierOrder defines the canonical token order so "shift+ctrl+k" and
// "ctrl+shift+k" hash to the same key.
var modifierOrder = map[string]int{
	"ctrl":  0,
	"alt":   1,
	"shift": 2,
}

// keyAliases maps friendly names to the strings tea.KeyMsg.String() actually
// emits. Anything not present is passed through verbatim — runes like "x" or
// "?" are valid keys.
var keyAliases = map[string]string{
	"return":    "enter",
	"esc":       "esc",
	"escape":    "esc",
	"space":     "space",
	"spacebar":  "space",
	"tab":       "tab",
	"backspace": "backspace",
	"delete":    "delete",
	"del":       "delete",
	"up":        "up",
	"down":      "down",
	"left":      "left",
	"right":     "right",
	"pgup":      "pgup",
	"pageup":    "pgup",
	"pgdown":    "pgdown",
	"pagedown":  "pgdown",
	"home":      "home",
	"end":       "end",
}

// NormalizeKey returns the canonical form of key. Sequences of two chords
// are supported by separating them with a single space: "ctrl+x ctrl+s".
func NormalizeKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("empty key")
	}
	// Sequence: split on spaces, normalise each chord, rejoin with " ".
	chords := strings.Fields(key)
	if len(chords) == 0 {
		return "", fmt.Errorf("empty key")
	}
	out := make([]string, 0, len(chords))
	for _, c := range chords {
		norm, err := normaliseChord(c)
		if err != nil {
			return "", err
		}
		out = append(out, norm)
	}
	return strings.Join(out, " "), nil
}

func normaliseChord(chord string) (string, error) {
	chord = strings.TrimSpace(chord)
	if chord == "" {
		return "", fmt.Errorf("empty chord")
	}
	parts := strings.Split(chord, "+")
	if len(parts) == 1 && parts[0] == "" {
		return "", fmt.Errorf("empty chord")
	}

	mods := map[string]bool{}
	var base string
	for i, raw := range parts {
		token := strings.ToLower(strings.TrimSpace(raw))
		if token == "" {
			// "ctrl+" or "++" type input — treat as malformed.
			return "", fmt.Errorf("empty token in chord %q", chord)
		}
		if canonical, ok := validModifiers[token]; ok && i < len(parts)-1 {
			mods[canonical] = true
			continue
		}
		// The last token is the base key. Resolve aliases; otherwise pass
		// through (single rune like "x", "?", or named like "f1").
		if alias, ok := keyAliases[token]; ok {
			base = alias
		} else {
			base = token
		}
	}
	if base == "" {
		return "", fmt.Errorf("chord %q has no base key", chord)
	}

	// Order modifiers canonically.
	ordered := make([]string, 0, len(mods))
	for m := range mods {
		ordered = append(ordered, m)
	}
	sortByOrder(ordered)
	if len(ordered) == 0 {
		return base, nil
	}
	return strings.Join(ordered, "+") + "+" + base, nil
}

func sortByOrder(s []string) {
	// Tiny hand-rolled insertion sort keeps this allocation-free for the
	// at-most-3-element slice we ever pass in.
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && modifierOrder[s[j-1]] > modifierOrder[s[j]] {
			s[j-1], s[j] = s[j], s[j-1]
			j--
		}
	}
}
