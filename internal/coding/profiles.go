// Package coding contains the coding agent REPL and related types.
package coding

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// AgentProfileSource identifies whether a profile came from the user-global
// or project-local directory. Project profiles shadow user profiles of the
// same name.
type AgentProfileSource string

const (
	AgentProfileSourceUser    AgentProfileSource = "user"
	AgentProfileSourceProject AgentProfileSource = "project"
)

// AgentProfile is a named agent persona loaded from a markdown file with
// YAML frontmatter. Profiles live at ~/.conduit/agents/*.md (user-global)
// and .conduit/agents/*.md (project-local). Project profiles override user
// profiles by name.
type AgentProfile struct {
	// Name is the unique identifier. Derived from frontmatter; falls back
	// to the filename stem when the frontmatter field is absent.
	Name string

	// Description explains what the agent does (frontmatter or body text).
	Description string

	// Model overrides the active provider model for this agent. Empty means
	// use the session default.
	Model string

	// Tools is the explicit allowlist of tool names for this agent. Nil means
	// inherit all tools registered for the session.
	Tools []string

	// InitialPrompt is prepended to the first user turn as a system prefix.
	InitialPrompt string

	// Source records whether this profile came from the user or project dir.
	Source AgentProfileSource

	// Path is the absolute filesystem path of the source .md file.
	Path string
}

// profileFrontmatter is the YAML header parsed from an agent profile .md file.
type profileFrontmatter struct {
	Name          string   `yaml:"name"`
	Description   string   `yaml:"description"`
	Model         string   `yaml:"model"`
	Tools         []string `yaml:"tools"`
	InitialPrompt string   `yaml:"initialPrompt"`
}

// DefaultAgentProfileDirs returns the canonical user and project agent
// profile directories for the given home and workspace paths.
func DefaultAgentProfileDirs(home, workspace string) (userDir, projectDir string) {
	if home != "" {
		userDir = filepath.Join(home, ".conduit", "agents")
	}
	if workspace != "" {
		projectDir = filepath.Join(workspace, ".conduit", "agents")
	}
	return userDir, projectDir
}

// LoadProfiles reads all *.md agent profiles from userDir and projectDir.
// Project profiles override user profiles with the same name. Either
// directory may be absent; missing directories are silently skipped.
func LoadProfiles(userDir, projectDir string) ([]AgentProfile, error) {
	byName := make(map[string]AgentProfile)

	for _, entry := range []struct {
		dir    string
		source AgentProfileSource
	}{
		{userDir, AgentProfileSourceUser},
		{projectDir, AgentProfileSourceProject},
	} {
		if entry.dir == "" {
			continue
		}
		profiles, err := loadProfileDir(entry.dir, entry.source)
		if err != nil {
			return nil, err
		}
		for _, p := range profiles {
			byName[p.Name] = p
		}
	}

	out := make([]AgentProfile, 0, len(byName))
	for _, p := range byName {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// loadProfileDir reads every *.md file in dir and parses it as an AgentProfile.
func loadProfileDir(dir string, source AgentProfileSource) ([]AgentProfile, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("agents: stat %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("agents: read dir %q: %w", dir, err)
	}

	var profiles []AgentProfile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("agents: read %q: %w", path, err)
		}
		p, err := parseProfile(path, data, source)
		if err != nil {
			// Skip unparseable files rather than aborting the sweep.
			continue
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}

// parseProfile converts raw .md bytes into an AgentProfile.
func parseProfile(path string, data []byte, source AgentProfileSource) (AgentProfile, error) {
	front, body, err := splitProfileFrontmatter(data)
	if err != nil {
		return AgentProfile{}, fmt.Errorf("agents: parse %q: %w", path, err)
	}

	name := strings.TrimSpace(front.Name)
	if name == "" {
		base := filepath.Base(path)
		name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return AgentProfile{}, fmt.Errorf("agents: %q has no usable name", path)
	}

	desc := strings.TrimSpace(front.Description)
	if desc == "" {
		desc = strings.TrimSpace(body)
	}

	return AgentProfile{
		Name:          name,
		Description:   desc,
		Model:         strings.TrimSpace(front.Model),
		Tools:         front.Tools,
		InitialPrompt: strings.TrimSpace(front.InitialPrompt),
		Source:        source,
		Path:          path,
	}, nil
}

// splitProfileFrontmatter extracts the YAML header and body from a .md file.
// Returns zero-value frontmatter and the raw content when no --- fence is found.
func splitProfileFrontmatter(data []byte) (profileFrontmatter, string, error) {
	normalised := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(normalised, []byte("---\n")) {
		return profileFrontmatter{}, string(normalised), nil
	}

	rest := normalised[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return profileFrontmatter{}, string(normalised), nil
	}
	header := rest[:end]
	body := rest[end+len("\n---"):]
	body = bytes.TrimPrefix(body, []byte("\n"))

	var front profileFrontmatter
	if len(bytes.TrimSpace(header)) > 0 {
		if err := yaml.Unmarshal(header, &front); err != nil {
			return profileFrontmatter{}, "", err
		}
	}
	return front, string(body), nil
}

// WriteProfile serialises an AgentProfile to a .md file in the target directory,
// creating the directory if needed. The filename is derived from the profile name.
func WriteProfile(dir string, p AgentProfile) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("agents: mkdir %q: %w", dir, err)
	}

	front := profileFrontmatter{
		Name:          p.Name,
		Description:   p.Description,
		Model:         p.Model,
		Tools:         p.Tools,
		InitialPrompt: p.InitialPrompt,
	}
	frontBytes, err := yaml.Marshal(front)
	if err != nil {
		return fmt.Errorf("agents: marshal frontmatter: %w", err)
	}

	content := "---\n" + string(frontBytes) + "---\n"

	filename := sanitiseProfileName(p.Name) + ".md"
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("agents: write %q: %w", path, err)
	}
	return nil
}

// DeleteProfile removes the .md file for the named profile from dir.
// Returns an error if the file does not exist.
func DeleteProfile(dir, name string) error {
	filename := sanitiseProfileName(name) + ".md"
	path := filepath.Join(dir, filename)
	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("agents: profile %q not found in %s", name, dir)
		}
		return fmt.Errorf("agents: delete %q: %w", path, err)
	}
	return nil
}

// sanitiseProfileName converts a profile name to a safe filename stem.
func sanitiseProfileName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == ' ' || r == '_':
			b.WriteRune('-')
		}
	}
	return b.String()
}
