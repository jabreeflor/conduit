package eval

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// DefaultResultsDir returns ~/.conduit/evals/results.
func DefaultResultsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, defaultResultsDir), nil
}

// Store appends and reads eval case results as JSONL.
type Store struct {
	Dir string
}

// NewStore creates a Store rooted at dir, or the default dir when empty.
func NewStore(dir string) (Store, error) {
	if dir == "" {
		var err error
		dir, err = DefaultResultsDir()
		if err != nil {
			return Store{}, err
		}
	}
	return Store{Dir: dir}, nil
}

// Append writes results to a run-scoped JSONL file and returns its path.
func (s Store) Append(results []CaseResult) (string, error) {
	if len(results) == 0 {
		return "", errors.New("eval: no results to store")
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return "", fmt.Errorf("eval: create results dir: %w", err)
	}
	path := filepath.Join(s.Dir, results[0].RunID+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("eval: open results: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, result := range results {
		if err := enc.Encode(result); err != nil {
			return "", fmt.Errorf("eval: encode result: %w", err)
		}
	}
	return path, nil
}

// ReadAll returns all stored results, newest files first.
func (s Store) ReadAll() ([]CaseResult, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("eval: read results dir: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() > entries[j].Name()
	})
	var out []CaseResult
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		results, err := readResultFile(filepath.Join(s.Dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, results...)
	}
	return out, nil
}

func readResultFile(path string) ([]CaseResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("eval: open %s: %w", path, err)
	}
	defer f.Close()

	var out []CaseResult
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var result CaseResult
		if err := json.Unmarshal(scanner.Bytes(), &result); err != nil {
			return nil, fmt.Errorf("eval: parse %s: %w", path, err)
		}
		out = append(out, result)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("eval: scan %s: %w", path, err)
	}
	return out, nil
}

// FilterResults narrows stored results for report flags.
func FilterResults(results []CaseResult, model string, since time.Time) []CaseResult {
	var out []CaseResult
	for _, result := range results {
		if model != "" && result.Model != model {
			continue
		}
		if !since.IsZero() && result.At.Before(since) {
			continue
		}
		out = append(out, result)
	}
	return out
}
