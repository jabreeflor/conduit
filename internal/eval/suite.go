package eval

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadSuites loads one suite file or every .yaml/.yml file under a directory.
func LoadSuites(path string) ([]Suite, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("eval: stat %s: %w", path, err)
	}
	if !info.IsDir() {
		suite, err := LoadSuiteFile(path)
		if err != nil {
			return nil, err
		}
		return []Suite{suite}, nil
	}

	var paths []string
	if err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if ext == ".yaml" || ext == ".yml" {
			paths = append(paths, p)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("eval: walk %s: %w", path, err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("eval: no YAML suites found in %s", path)
	}

	suites := make([]Suite, 0, len(paths))
	for _, p := range paths {
		suite, err := LoadSuiteFile(p)
		if err != nil {
			return nil, err
		}
		suites = append(suites, suite)
	}
	return suites, nil
}

// LoadSuiteFile parses and validates a custom eval suite.
func LoadSuiteFile(path string) (Suite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Suite{}, fmt.Errorf("eval: read %s: %w", path, err)
	}
	var suite Suite
	if err := yaml.Unmarshal(data, &suite); err != nil {
		return Suite{}, fmt.Errorf("eval: parse %s: %w", path, err)
	}
	if err := suite.Validate(); err != nil {
		return Suite{}, fmt.Errorf("eval: %s: %w", path, err)
	}
	return suite, nil
}

// Validate checks the minimum shape needed to run a suite.
func (s Suite) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("suite name is required")
	}
	if len(s.Cases) == 0 {
		return errors.New("at least one case is required")
	}
	for i, c := range s.Cases {
		if strings.TrimSpace(c.Name) == "" {
			return fmt.Errorf("case %d: name is required", i+1)
		}
		if strings.TrimSpace(c.Input) == "" {
			return fmt.Errorf("case %q: input is required", c.Name)
		}
		if !hasAssertions(c.Expect) {
			return fmt.Errorf("case %q: at least one expectation is required", c.Name)
		}
	}
	return nil
}

func hasAssertions(e Expectations) bool {
	return len(e.ToolCallsInclude) > 0 ||
		len(e.ToolCallsExclude) > 0 ||
		e.ReplyContains != "" ||
		e.ReplyContainsTag != "" ||
		e.ReplySentiment != "" ||
		e.DurationMaxSeconds > 0 ||
		e.CostMaxUSD > 0 ||
		e.NoPromptInjectionDetected != nil ||
		e.WorkflowStepsCompleted != "" ||
		e.ContextRetained != ""
}
