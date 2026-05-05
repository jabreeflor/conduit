package skills

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// MockModelCaller implements the ModelCaller interface for testing.
type MockModelCaller struct {
	callCount int
	response  string
	err       error
}

func (m *MockModelCaller) Call(prompt string) (string, error) {
	m.callCount++
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestShouldGenerate(t *testing.T) {
	tests := []struct {
		name     string
		summary  SessionSummary
		expected bool
	}{
		{
			name: "successful session with sufficient data",
			summary: SessionSummary{
				SessionID:       "sess-123",
				TaskDescription: "Extract data from a CSV file using Go",
				Turns:           5,
				Outcome:         "success",
				ToolsUsed:       []string{"file-reader", "parser"},
				Duration:        10 * time.Minute,
			},
			expected: true,
		},
		{
			name: "failed outcome",
			summary: SessionSummary{
				TaskDescription: "Extract data from a CSV file",
				Turns:           5,
				Outcome:         "failed",
				ToolsUsed:       []string{"file-reader"},
			},
			expected: false,
		},
		{
			name: "partial outcome",
			summary: SessionSummary{
				TaskDescription: "Extract data from a CSV file",
				Turns:           5,
				Outcome:         "partial",
				ToolsUsed:       []string{"file-reader"},
			},
			expected: false,
		},
		{
			name: "insufficient turns (< 3)",
			summary: SessionSummary{
				TaskDescription: "Extract data from a CSV file",
				Turns:           2,
				Outcome:         "success",
				ToolsUsed:       []string{"file-reader"},
			},
			expected: false,
		},
		{
			name: "no tools used",
			summary: SessionSummary{
				TaskDescription: "Extract data from a CSV file",
				Turns:           5,
				Outcome:         "success",
				ToolsUsed:       []string{},
			},
			expected: false,
		},
		{
			name: "empty task description",
			summary: SessionSummary{
				TaskDescription: "",
				Turns:           5,
				Outcome:         "success",
				ToolsUsed:       []string{"tool"},
			},
			expected: false,
		},
		{
			name: "task description too short",
			summary: SessionSummary{
				TaskDescription: "short",
				Turns:           5,
				Outcome:         "success",
				ToolsUsed:       []string{"tool"},
			},
			expected: false,
		},
		{
			name: "whitespace-only task description",
			summary: SessionSummary{
				TaskDescription: "   ",
				Turns:           5,
				Outcome:         "success",
				ToolsUsed:       []string{"tool"},
			},
			expected: false,
		},
		{
			name: "exactly 3 turns",
			summary: SessionSummary{
				TaskDescription: "Extract data from CSV file",
				Turns:           3,
				Outcome:         "success",
				ToolsUsed:       []string{"tool"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ag := NewAutoGenerator(&MockModelCaller{})
			result := ag.ShouldGenerate(tt.summary)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGenerate(t *testing.T) {
	validSkillMarkdown := `---
name: extract csv data
description: Extract and parse data from CSV files
tags: [csv, parsing, data-extraction]
---

## Overview
This skill demonstrates how to efficiently extract and parse CSV data using Go's standard library.

## Steps
1. Open the CSV file
2. Parse rows
3. Process data

## Example
Use the csv.Reader for efficient parsing.
`

	tests := []struct {
		name            string
		summary         SessionSummary
		mockResponse    string
		mockErr         error
		shouldSucceed   bool
		validateContent func(*testing.T, contracts.Skill)
	}{
		{
			name: "successful generation with frontmatter",
			summary: SessionSummary{
				SessionID:       "sess-123",
				TaskDescription: "Extract data from a CSV file using Go",
				Turns:           5,
				Outcome:         "success",
				ToolsUsed:       []string{"file-reader", "parser"},
				Duration:        10 * time.Minute,
			},
			mockResponse:  validSkillMarkdown,
			shouldSucceed: true,
			validateContent: func(t *testing.T, skill contracts.Skill) {
				if skill.Name == "" {
					t.Error("skill name should not be empty")
				}
				if skill.Tier != contracts.SkillTierPersonal {
					t.Errorf("expected SkillTierPersonal, got %v", skill.Tier)
				}
				if skill.Description == "" {
					t.Error("skill description should not be empty")
				}
				if skill.Body == "" {
					t.Error("skill body should not be empty")
				}
			},
		},
		{
			name: "generation without frontmatter",
			summary: SessionSummary{
				SessionID:       "sess-456",
				TaskDescription: "Parse JSON data efficiently",
				Turns:           4,
				Outcome:         "success",
				ToolsUsed:       []string{"json-parser"},
				Duration:        5 * time.Minute,
			},
			mockResponse:  "This is a skill without frontmatter\n\nJust markdown content.",
			shouldSucceed: true,
			validateContent: func(t *testing.T, skill contracts.Skill) {
				if skill.Name == "" {
					t.Error("skill name should be generated from task")
				}
				if skill.Description != "Parse JSON data efficiently" {
					t.Error("description should fall back to task description")
				}
			},
		},
		{
			name: "model returns error",
			summary: SessionSummary{
				TaskDescription: "Some task",
				Turns:           5,
				Outcome:         "success",
				ToolsUsed:       []string{"tool"},
			},
			mockErr:       fmt.Errorf("model unavailable"),
			shouldSucceed: false,
		},
		{
			name: "session does not meet generation criteria",
			summary: SessionSummary{
				TaskDescription: "task",
				Turns:           2,
				Outcome:         "failed",
				ToolsUsed:       []string{},
			},
			mockResponse:  "",
			shouldSucceed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockModelCaller{
				response: tt.mockResponse,
				err:      tt.mockErr,
			}
			ag := NewAutoGenerator(mock)

			skill, err := ag.Generate(tt.summary)

			if tt.shouldSucceed {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				if tt.validateContent != nil {
					tt.validateContent(t, skill)
				}
			} else {
				if err == nil {
					t.Errorf("expected error, got none")
				}
			}
		})
	}
}

func TestSave(t *testing.T) {
	tests := []struct {
		name          string
		skill         contracts.Skill
		skillsDir     string
		shouldSucceed bool
	}{
		{
			name: "save valid skill",
			skill: contracts.Skill{
				Name:        "Test Skill",
				Description: "A test skill",
				Tags:        []string{"test", "example"},
				Body:        "This is the skill body",
				Tier:        contracts.SkillTierPersonal,
			},
			skillsDir:     "/tmp/test-skills",
			shouldSucceed: true,
		},
		{
			name: "empty skills directory",
			skill: contracts.Skill{
				Name: "Test Skill",
				Body: "Body",
			},
			skillsDir:     "",
			shouldSucceed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ag := NewAutoGenerator(&MockModelCaller{})
			path, err := ag.Save(tt.skill, tt.skillsDir)

			if tt.shouldSucceed {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				if path == "" {
					t.Error("expected non-empty path")
				}
				if !strings.Contains(path, tt.skillsDir) {
					t.Errorf("path %q should contain directory %q", path, tt.skillsDir)
				}
			} else {
				if err == nil {
					t.Errorf("expected error, got none")
				}
			}
		})
	}
}

func TestGenerateSkillName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "Extract data from a CSV file using Go",
			expected: "Extract-Data-From-A-Csv",
		},
		{
			input:    "parse json data efficiently",
			expected: "Parse-Json-Data-Efficiently",
		},
		{
			input:    "simple",
			expected: "Simple",
		},
		{
			input:    "task with special!@#$%chars",
			expected: "Task-With-Special-chars",
		},
		{
			input:    "multiple   spaces between words",
			expected: "Multiple-Spaces-Between",
		},
		{
			input:    "trailing punctuation...",
			expected: "Trailing-Punctuation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := generateSkillName(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "Test Skill Name",
			expected: "test-skill-name",
		},
		{
			input:    "UPPERCASE NAME",
			expected: "uppercase-name",
		},
		{
			input:    "name-with-dashes",
			expected: "name-with-dashes",
		},
		{
			input:    "name!@#$%with^special&chars",
			expected: "namewithspecialchars",
		},
		{
			input:    "  spaces  around  ",
			expected: "spaces-around",
		},
		{
			input:    "single",
			expected: "single",
		},
		{
			input:    "---dashes---",
			expected: "dashes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeFilename(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestExtractFrontmatter(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectedName    string
		expectedDesc    string
		expectedTags    []string
		expectedBody    string
		shouldError     bool
	}{
		{
			name: "valid frontmatter with all fields",
			input: `---
name: test-skill
description: This is a test skill
tags: [test, example, sample]
---

This is the body content.
It spans multiple lines.`,
			expectedName: "test-skill",
			expectedDesc: "This is a test skill",
			expectedTags: []string{"test", "example", "sample"},
			expectedBody: "This is the body content.\nIt spans multiple lines.",
		},
		{
			name: "no frontmatter",
			input: `Just plain markdown content
without any YAML frontmatter`,
			expectedBody: "Just plain markdown content\nwithout any YAML frontmatter",
		},
		{
			name: "incomplete frontmatter (no closing ---)",
			input: `---
name: incomplete
description: missing closing fence

Body content here`,
			expectedBody: "Body content here",
		},
		{
			name: "empty frontmatter",
			input: `---
---

Body only`,
			expectedBody: "Body only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			front, body, err := extractFrontmatter(tt.input)

			if tt.shouldError && err == nil {
				t.Error("expected error, got none")
				return
			}
			if !tt.shouldError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if front.Name != tt.expectedName {
				t.Errorf("name: expected %q, got %q", tt.expectedName, front.Name)
			}
			if front.Description != tt.expectedDesc {
				t.Errorf("description: expected %q, got %q", tt.expectedDesc, front.Description)
			}
			if body != tt.expectedBody {
				t.Errorf("body: expected %q, got %q", tt.expectedBody, body)
			}
		})
	}
}

func TestNormaliseTags(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "normal tags",
			input:    []string{"tag1", "tag2", "tag3"},
			expected: []string{"tag1", "tag2", "tag3"},
		},
		{
			name:     "tags with whitespace",
			input:    []string{"  tag1  ", " tag2 ", "tag3"},
			expected: []string{"tag1", "tag2", "tag3"},
		},
		{
			name:     "empty string in tags",
			input:    []string{"tag1", "", "tag2"},
			expected: []string{"tag1", "tag2"},
		},
		{
			name:     "all empty",
			input:    []string{"", "  ", ""},
			expected: nil,
		},
		{
			name:     "empty input",
			input:    []string{},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normaliseTags(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("length mismatch: expected %d, got %d", len(tt.expected), len(result))
				return
			}
			for i, tag := range result {
				if tag != tt.expected[i] {
					t.Errorf("tag %d: expected %q, got %q", i, tt.expected[i], tag)
				}
			}
		})
	}
}

func TestAutoGeneratorIntegration(t *testing.T) {
	// Test the full workflow: should generate -> generate -> save
	mockCaller := &MockModelCaller{
		response: `---
name: process-csv-files
description: Process and analyze CSV files efficiently
tags: [csv, data, processing]
---

## How to Process CSV Files

This skill shows efficient CSV processing patterns in Go.

### Key Steps
1. Use csv.Reader for parsing
2. Handle errors gracefully
3. Process records in batches
`,
	}

	ag := NewAutoGenerator(mockCaller)

	summary := SessionSummary{
		SessionID:       "integration-test-123",
		TaskDescription: "Process and analyze CSV files with Go",
		Turns:           7,
		Outcome:         "success",
		ToolsUsed:       []string{"csv-reader", "file-processor", "data-analyzer"},
		Duration:        25 * time.Minute,
	}

	// Check if we should generate
	if !ag.ShouldGenerate(summary) {
		t.Fatal("ShouldGenerate returned false for valid session")
	}

	// Generate the skill
	skill, err := ag.Generate(summary)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if skill.Name == "" {
		t.Error("generated skill has empty name")
	}
	if skill.Body == "" {
		t.Error("generated skill has empty body")
	}
	if skill.Tier != contracts.SkillTierPersonal {
		t.Errorf("unexpected tier: %v", skill.Tier)
	}

	// Model should have been called
	if mockCaller.callCount != 1 {
		t.Errorf("expected 1 model call, got %d", mockCaller.callCount)
	}
}
