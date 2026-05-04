package changelog

import (
	"strings"
	"testing"
)

func TestParseSplitsSections(t *testing.T) {
	src := `# Changelog

## [Unreleased]

_pending_

## [v0.2.0] - 2026-04-15

### Added
- thing one (#1)

### Fixed
- thing two (#2)

## [v0.1.0] - 2026-03-01

### Added
- initial release (#0)
`
	got := parse(src)
	if len(got) != 3 {
		t.Fatalf("want 3 sections, got %d: %#v", len(got), got)
	}
	if got[0].Heading != "Unreleased" {
		t.Errorf("section[0] heading = %q", got[0].Heading)
	}
	if got[1].Heading != "v0.2.0" {
		t.Errorf("section[1] heading = %q", got[1].Heading)
	}
	if got[2].Heading != "v0.1.0" {
		t.Errorf("section[2] heading = %q", got[2].Heading)
	}
	if !strings.Contains(got[1].Body, "thing one") {
		t.Errorf("section[1] body missing bullet: %q", got[1].Body)
	}
}

func TestLatestSkipsUnreleased(t *testing.T) {
	old := raw
	defer func() { raw = old }()
	raw = `# Changelog

## [Unreleased]

_pending_

## [v0.2.0] - 2026-04-15

### Added
- thing (#1)
`
	got := Latest()
	if got.Heading != "v0.2.0" {
		t.Errorf("Latest() heading = %q, want v0.2.0", got.Heading)
	}
	if !strings.Contains(got.Body, "thing") {
		t.Errorf("Latest() body missing bullet")
	}
}

func TestLatestFallsBackToUnreleasedWhenNoReleases(t *testing.T) {
	old := raw
	defer func() { raw = old }()
	raw = `# Changelog

## [Unreleased]

_pending_
`
	got := Latest()
	if got.Heading != "Unreleased" {
		t.Errorf("Latest() heading = %q, want Unreleased", got.Heading)
	}
}

func TestShouldShow(t *testing.T) {
	cases := []struct {
		name     string
		lastSeen string
		current  string
		want     bool
	}{
		{"fresh install", "", "v0.2.0", true},
		{"upgrade", "v0.1.0", "v0.2.0", true},
		{"same version", "v0.2.0", "v0.2.0", false},
		{"whitespace ignored", " v0.2.0\n", "v0.2.0", false},
		{"empty current never shows", "v0.1.0", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldShow(tc.lastSeen, tc.current); got != tc.want {
				t.Errorf("ShouldShow(%q, %q) = %v, want %v", tc.lastSeen, tc.current, got, tc.want)
			}
		})
	}
}

func TestRawIsNonEmpty(t *testing.T) {
	if strings.TrimSpace(Raw()) == "" {
		t.Fatal("embedded changelog is empty")
	}
}
