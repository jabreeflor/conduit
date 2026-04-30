package design

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// WCAGRatio returns the contrast ratio between two hex colors per WCAG 2.1.
// Hex strings may include or omit the leading '#'. Result is in [1, 21].
func WCAGRatio(fg, bg string) (float64, error) {
	l1, err := relativeLuminance(fg)
	if err != nil {
		return 0, fmt.Errorf("fg %q: %w", fg, err)
	}
	l2, err := relativeLuminance(bg)
	if err != nil {
		return 0, fmt.Errorf("bg %q: %w", bg, err)
	}
	light, dark := l1, l2
	if dark > light {
		light, dark = dark, light
	}
	return (light + 0.05) / (dark + 0.05), nil
}

func relativeLuminance(hex string) (float64, error) {
	r, g, b, err := parseHex(hex)
	if err != nil {
		return 0, err
	}
	return 0.2126*channel(r) + 0.7152*channel(g) + 0.0722*channel(b), nil
}

func channel(c float64) float64 {
	if c <= 0.03928 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func parseHex(s string) (r, g, b float64, err error) {
	s = strings.TrimSpace(strings.TrimPrefix(s, "#"))
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return 0, 0, 0, fmt.Errorf("expected 6-digit hex, got %q", s)
	}
	parse := func(c string) (float64, error) {
		v, err := strconv.ParseUint(c, 16, 8)
		if err != nil {
			return 0, err
		}
		return float64(v) / 255, nil
	}
	if r, err = parse(s[0:2]); err != nil {
		return 0, 0, 0, err
	}
	if g, err = parse(s[2:4]); err != nil {
		return 0, 0, 0, err
	}
	if b, err = parse(s[4:6]); err != nil {
		return 0, 0, 0, err
	}
	return r, g, b, nil
}
