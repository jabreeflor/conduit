package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
	"gopkg.in/yaml.v3"
)

// ModelCaller is the interface for calling AI models during skill generation.
type ModelCaller interface {
	// Call sends a prompt to the model and returns the generated skill body.
	// The caller is responsible for ensuring the returned text is valid markdown.
	Call(prompt string) (string, error)
}

// SessionSummary contains session metadata needed for skill generation.
type SessionSummary struct {
	SessionID      string
	TaskDescription string
	Turns          int
	Outcome        string // "success", "partial", "failed"
	Duration       time.Duration
	ToolsUsed      []string
	WorkflowType   string
}

// AutoGenerator creates a skill from a session summary using an AI model.
type AutoGenerator struct {
	caller ModelCaller
}

// NewAutoGenerator returns a new AutoGenerator with the given ModelCaller.
func NewAutoGenerator(caller ModelCaller) *AutoGenerator {
	return &AutoGenerator{caller: caller}
}

// ShouldGenerate determines if a skill should be auto-generated from a session.
// Returns true if the session meets criteria for skill generation (e.g., success,
// non-trivial number of turns, distinct task type).
func (ag *AutoGenerator) ShouldGenerate(summary SessionSummary) bool {
	// Only generate skills for successful sessions
	if summary.Outcome != "success" {
		return false
	}

	// Require at least 3 turns (user, assistant, at least one substantive exchange)
	if summary.Turns < 3 {
		return false
	}

	// Require at least one tool used
	if len(summary.ToolsUsed) == 0 {
		return false
	}

	// Require a meaningful task description
	taskDesc := strings.TrimSpace(summary.TaskDescription)
	if taskDesc == "" || len(taskDesc) < 10 {
		return false
	}

	return true
}

// Generate creates a skill from a session summary by calling the model.
// Returns a Skill struct suitable for saving to disk.
func (ag *AutoGenerator) Generate(summary SessionSummary) (contracts.Skill, error) {
	if !ag.ShouldGenerate(summary) {
		return contracts.Skill{}, fmt.Errorf("session does not meet skill generation criteria")
	}

	// Build prompt for skill generation
	prompt := ag.buildGenerationPrompt(summary)

	// Call the model to generate skill body
	body, err := ag.caller.Call(prompt)
	if err != nil {
		return contracts.Skill{}, fmt.Errorf("model call failed: %w", err)
	}

	// Parse frontmatter and extract metadata
	frontMatter, bodyText, err := extractFrontmatter(body)
	if err != nil {
		return contracts.Skill{}, fmt.Errorf("parse generated skill: %w", err)
	}

	// Generate skill name from task description
	skillName := generateSkillName(summary.TaskDescription)
	if frontMatter.Name != "" {
		skillName = frontMatter.Name
	}

	// Extract description from frontmatter or task
	description := frontMatter.Description
	if description == "" {
		description = summary.TaskDescription
		if len(description) > 200 {
			description = description[:197] + "..."
		}
	}

	skill := contracts.Skill{
		Name:        skillName,
		Tier:        contracts.SkillTierPersonal,
		Description: description,
		Tags:        normaliseTags(frontMatter.Tags),
		Body:        bodyText,
		Path:        "", // Will be set by Save()
		UpdatedAt:   time.Now(),
	}

	return skill, nil
}

// Save persists a skill to disk in the personal skills directory.
// Creates the skill file with YAML frontmatter and returns the path.
func (ag *AutoGenerator) Save(skill contracts.Skill, skillsDir string) (string, error) {
	if skillsDir == "" {
		return "", fmt.Errorf("skills directory not specified")
	}

	// Ensure the skills directory exists
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return "", fmt.Errorf("create skills directory: %w", err)
	}

	// Generate filename from skill name
	filename := sanitizeFilename(skill.Name) + ".md"
	skillPath := filepath.Join(skillsDir, filename)

	// Build file content with YAML frontmatter
	fileContent := ag.buildFileContent(skill)

	// Write to disk
	if err := os.WriteFile(skillPath, []byte(fileContent), 0644); err != nil {
		return "", fmt.Errorf("write skill file: %w", err)
	}

	return skillPath, nil
}

// buildGenerationPrompt creates the prompt for the model to generate a skill.
func (ag *AutoGenerator) buildGenerationPrompt(summary SessionSummary) string {
	toolsStr := strings.Join(summary.ToolsUsed, ", ")
	if toolsStr == "" {
		toolsStr = "various"
	}

	prompt := fmt.Sprintf(`You are a skill documentation expert. Create a reusable skill from this session summary.

Session Details:
- Task: %s
- Outcome: %s
- Turns: %d
- Tools Used: %s
- Workflow Type: %s
- Duration: %s

Generate a markdown skill file with YAML frontmatter. The skill should be reusable for similar tasks.

Format:
---
name: <concise skill name>
description: <one-line summary of what this skill does>
tags: [<relevant>, <tags>]
---

<skill body with detailed instructions and examples>

Make the skill practical and immediately useful.
`, summary.TaskDescription, summary.Outcome, summary.Turns, toolsStr, summary.WorkflowType, summary.Duration.String())

	return prompt
}

// buildFileContent constructs the complete file content with frontmatter.
func (ag *AutoGenerator) buildFileContent(skill contracts.Skill) string {
	frontMatter := map[string]interface{}{
		"name":        skill.Name,
		"description": skill.Description,
	}
	if len(skill.Tags) > 0 {
		frontMatter["tags"] = skill.Tags
	}

	// Marshal frontmatter to YAML
	yamlBytes, _ := yaml.Marshal(frontMatter)
	yamlStr := string(yamlBytes)

	return fmt.Sprintf("---%s---\n%s\n", yamlStr, strings.TrimSpace(skill.Body))
}

// extractFrontmatter parses YAML frontmatter from generated skill content.
// Returns the parsed frontmatter, remaining body, and any error.
func extractFrontmatter(content string) (markdownFrontmatter, string, error) {
	lines := strings.SplitN(content, "\n", 2)
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "---") {
		return markdownFrontmatter{}, content, nil
	}

	if len(lines) == 1 {
		return markdownFrontmatter{}, "", nil
	}

	rest := lines[1]
	endIdx := strings.Index(rest, "---")
	if endIdx < 0 {
		return markdownFrontmatter{}, rest, nil
	}

	headerStr := rest[:endIdx]
	bodyStr := strings.TrimPrefix(rest[endIdx+3:], "\n")

	var front markdownFrontmatter
	if headerStr != "" {
		if err := yaml.Unmarshal([]byte(headerStr), &front); err != nil {
			return markdownFrontmatter{}, "", err
		}
	}

	return front, bodyStr, nil
}

// generateSkillName creates a concise skill name from a task description.
func generateSkillName(taskDesc string) string {
	// Take first 40 characters, capitalize, remove special chars
	name := strings.ToLower(taskDesc)
	if len(name) > 40 {
		name = name[:40]
	}

	// Remove trailing punctuation
	name = strings.TrimRight(name, ".,!?;:")

	// Replace spaces and special chars with hyphens
	var result strings.Builder
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			result.WriteRune(ch)
		} else if ch == ' ' || ch == '-' {
			if result.Len() > 0 && result.String()[result.Len()-1] != '-' {
				result.WriteRune('-')
			}
		}
	}

	name = result.String()
	name = strings.Trim(name, "-")

	if name == "" {
		name = "auto-generated"
	}

	// Capitalize first letter of each word for display
	parts := strings.Split(name, "-")
	for i, part := range parts {
		if part != "" {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}

	return strings.Join(parts, " ")
}

// sanitizeFilename creates a safe filename from a skill name.
func sanitizeFilename(name string) string {
	// Convert to lowercase and replace spaces with hyphens
	filename := strings.ToLower(strings.TrimSpace(name))
	var result strings.Builder
	for _, ch := range filename {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			result.WriteRune(ch)
		} else if ch == ' ' || ch == '-' {
			if result.Len() > 0 && result.String()[result.Len()-1] != '-' {
				result.WriteRune('-')
			}
		}
	}
	return strings.Trim(result.String(), "-")
}
