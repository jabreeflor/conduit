// Package security contains protective checks for untrusted model context.
package security

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// ContentSource identifies where untrusted content came from before it is
// injected into a model prompt.
type ContentSource string

const (
	SourceMemory     ContentSource = "memory"
	SourceFile       ContentSource = "file"
	SourceWebFetch   ContentSource = "web_fetch"
	SourceToolOutput ContentSource = "tool_output"
)

// Finding describes one prompt-injection signal found in untrusted content.
type Finding struct {
	Source   ContentSource
	Pattern  string
	Category string
	Line     int
}

// ScanResult is the sanitized content plus the signals that caused changes.
type ScanResult struct {
	Source    ContentSource
	Original  string
	Sanitized string
	Findings  []Finding
}

// Detected reports whether the scanner found any suspicious content.
func (r ScanResult) Detected() bool {
	return len(r.Findings) > 0
}

type pattern struct {
	label    string
	category string
	re       *regexp.Regexp
}

var linePatterns = []pattern{
	{
		label:    "SYSTEM OVERRIDE",
		category: "instruction_override",
		re:       regexp.MustCompile(`(?i)\bsystem\s+override\b`),
	},
	{
		label:    "IGNORE INSTRUCTIONS",
		category: "instruction_override",
		re:       regexp.MustCompile(`(?i)\bignore\s+(all\s+)?(previous\s+|prior\s+|above\s+)?instructions\b`),
	},
	{
		label:    "PRETEND YOU ARE",
		category: "role_impersonation",
		re:       regexp.MustCompile(`(?i)\bpretend\s+you\s+are\b`),
	},
	{
		label:    "DISREGARD PREVIOUS",
		category: "instruction_override",
		re:       regexp.MustCompile(`(?i)\bdisregard\s+(all\s+)?(previous|prior|above)\b`),
	},
	{
		label:    "SHELL BACKTICKS",
		category: "shell_injection",
		re:       regexp.MustCompile("`[^`]+`"),
	},
	{
		label:    "SHELL FILE READ",
		category: "shell_injection",
		re:       regexp.MustCompile(`(?i)(^|[;&|]\s*)(cat|less)\s+(/|~|\.\./|[A-Za-z0-9_.-]+)`),
	},
	{
		label:    "SHELL REDIRECT",
		category: "shell_injection",
		re:       regexp.MustCompile(`(?:^|\s)(?:>{1,2}|<)\s*(?:/tmp|/var/tmp|~|\.\./|[A-Za-z0-9_.-]+)`),
	},
	{
		label:    "FILE EXFILTRATION",
		category: "file_exfiltration",
		re:       regexp.MustCompile(`(?i)\b(?:exfiltrate|upload|send|post|curl|wget)\b.*\b(?:/etc/passwd|\.ssh|id_rsa|api[_-]?key|secret|credential|token|env)\b`),
	},
}

var invisibleRunes = map[rune]string{
	'\u200B': "ZERO WIDTH SPACE",
	'\u200C': "ZERO WIDTH NON-JOINER",
	'\u200D': "ZERO WIDTH JOINER",
	'\u2060': "WORD JOINER",
	'\uFEFF': "ZERO WIDTH NO-BREAK SPACE",
	'\u202A': "LEFT-TO-RIGHT EMBEDDING",
	'\u202B': "RIGHT-TO-LEFT EMBEDDING",
	'\u202C': "POP DIRECTIONAL FORMATTING",
	'\u202D': "LEFT-TO-RIGHT OVERRIDE",
	'\u202E': "RIGHT-TO-LEFT OVERRIDE",
	'\u2066': "LEFT-TO-RIGHT ISOLATE",
	'\u2067': "RIGHT-TO-LEFT ISOLATE",
	'\u2068': "FIRST STRONG ISOLATE",
	'\u2069': "POP DIRECTIONAL ISOLATE",
}

// ScanInjectedContent scans content from an untrusted source and returns a
// sanitized string that is safe to pass deeper into prompt assembly.
func ScanInjectedContent(source ContentSource, content string) ScanResult {
	result := ScanResult{
		Source:    source,
		Original:  content,
		Sanitized: content,
	}

	lines := strings.SplitAfter(content, "\n")
	sanitizedLines := make([]string, 0, len(lines))

	for i, line := range lines {
		lineNumber := i + 1
		lineFindings := matchLine(source, lineNumber, line)
		if len(lineFindings) > 0 {
			result.Findings = append(result.Findings, lineFindings...)
			continue
		}

		sanitizedLine, invisibleFindings := stripInvisible(source, lineNumber, line)
		result.Findings = append(result.Findings, invisibleFindings...)
		sanitizedLines = append(sanitizedLines, sanitizedLine)
	}

	result.Sanitized = strings.Join(sanitizedLines, "")
	return result
}

func matchLine(source ContentSource, lineNumber int, line string) []Finding {
	findings := make([]Finding, 0, 1)
	for _, candidate := range linePatterns {
		if candidate.re.MatchString(line) {
			findings = append(findings, Finding{
				Source:   source,
				Pattern:  candidate.label,
				Category: candidate.category,
				Line:     lineNumber,
			})
		}
	}
	return findings
}

func stripInvisible(source ContentSource, lineNumber int, line string) (string, []Finding) {
	if utf8.RuneCountInString(line) == len(line) {
		return line, nil
	}

	var builder strings.Builder
	builder.Grow(len(line))
	findings := make([]Finding, 0, 1)
	seen := make(map[string]bool)

	for _, current := range line {
		label, ok := invisibleRunes[current]
		if !ok {
			builder.WriteRune(current)
			continue
		}

		if seen[label] {
			continue
		}
		seen[label] = true
		findings = append(findings, Finding{
			Source:   source,
			Pattern:  label,
			Category: "invisible_unicode",
			Line:     lineNumber,
		})
	}

	return builder.String(), findings
}
