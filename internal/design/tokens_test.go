package design

import (
	"path/filepath"
	"strings"
	"testing"
)

const sourcePath = "../../design/tokens.yaml"

func loadTestTokens(t *testing.T) *Tokens {
	t.Helper()
	abs, err := filepath.Abs(sourcePath)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	tk, err := Load(abs)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return tk
}

func TestLoadResolvesAllReferences(t *testing.T) {
	tk := loadTestTokens(t)
	for _, mode := range tk.Modes() {
		Walk(tk.Semantic[mode], func(path []string, value string) {
			if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
				t.Errorf("semantic.%s.%s: unresolved reference %s", mode, strings.Join(path, "."), value)
			}
		})
	}
}

func TestRequiredModesPresent(t *testing.T) {
	tk := loadTestTokens(t)
	for _, want := range []string{"dark", "light", "hc"} {
		if _, ok := tk.Semantic[want]; !ok {
			t.Errorf("missing required mode %q", want)
		}
	}
}

func TestWCAGRatioKnownPairs(t *testing.T) {
	cases := []struct {
		fg, bg string
		min    float64
	}{
		{"#FFFFFF", "#000000", 20.9},
		{"#000000", "#FFFFFF", 20.9},
		{"#777777", "#FFFFFF", 4.4},
	}
	for _, c := range cases {
		got, err := WCAGRatio(c.fg, c.bg)
		if err != nil {
			t.Fatalf("WCAGRatio(%s,%s): %v", c.fg, c.bg, err)
		}
		if got < c.min {
			t.Errorf("WCAGRatio(%s,%s) = %.2f, want >= %.2f", c.fg, c.bg, got, c.min)
		}
	}
}

// HighContrastModeMeetsAAA enforces the spec'd 7:1 minimum on every pair
// listed in `contrast_pairs_aaa` of the source file.
func TestHighContrastModeMeetsAAA(t *testing.T) {
	tk := loadTestTokens(t)
	hc, ok := tk.Semantic["hc"]
	if !ok {
		t.Fatal("hc mode missing")
	}
	color, ok := hc["color"].(Tree)
	if !ok {
		t.Fatal("hc.color missing")
	}
	if len(tk.ContrastPairsAAA) == 0 {
		t.Fatal("contrast_pairs_aaa is empty")
	}
	for _, pair := range tk.ContrastPairsAAA {
		fg := lookupColor(color, strings.Split(pair[0], ".")...)
		bg := lookupColor(color, strings.Split(pair[1], ".")...)
		if fg == "" || bg == "" {
			t.Errorf("pair (%s, %s): unresolved", pair[0], pair[1])
			continue
		}
		ratio, err := WCAGRatio(fg, bg)
		if err != nil {
			t.Errorf("pair (%s, %s): %v", pair[0], pair[1], err)
			continue
		}
		if ratio < 7.0 {
			t.Errorf("AAA fail: %s on %s = %s/%s ratio %.2f (need >= 7.0)", pair[0], pair[1], fg, bg, ratio)
		}
	}
}

func TestEmitCSSContainsRootAndModes(t *testing.T) {
	tk := loadTestTokens(t)
	out := EmitCSS(tk)
	for _, want := range []string{
		":root {",
		"[data-theme=\"light\"]",
		"[data-theme=\"hc\"]",
		"--color-surface-canvas:",
		"--color-brand-primary:",
		"--ref-color-saffron-300:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("EmitCSS output missing %q", want)
		}
	}
}

func TestEmitSwiftContainsThemeEnumAndPaths(t *testing.T) {
	tk := loadTestTokens(t)
	out := EmitSwift(tk)
	for _, want := range []string{
		"public enum ConduitTheme",
		"case dark",
		"case light",
		"case hc",
		"\"surface.canvas\":",
		"\"brand.primary\":",
		"Color(red:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("EmitSwift output missing %q", want)
		}
	}
}

func TestEmitTextualPerMode(t *testing.T) {
	tk := loadTestTokens(t)
	for _, mode := range []string{"dark", "light", "hc"} {
		out, err := EmitTextual(tk, mode)
		if err != nil {
			t.Fatalf("EmitTextual(%s): %v", mode, err)
		}
		if !strings.Contains(out, "\"name\": \"conduit-"+mode+"\"") {
			t.Errorf("EmitTextual(%s) missing name", mode)
		}
		if !strings.Contains(out, "\"background\":") {
			t.Errorf("EmitTextual(%s) missing background", mode)
		}
	}
}
