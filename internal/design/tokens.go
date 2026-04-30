// Package design loads, resolves, and emits Conduit design tokens.
//
// The token source (design/tokens.yaml) is split into a reference scale and
// per-mode semantic mappings. Loading produces a fully resolved Tokens value
// with every {dotted.path} reference replaced by its concrete scalar.
package design

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Tokens is the resolved token tree for a single source file.
type Tokens struct {
	Reference Tree            `yaml:"reference"`
	Semantic  map[string]Tree `yaml:"semantic"`
	// ContrastPairsAAA lists [foreground, background] semantic paths whose
	// resolved colors must meet WCAG AAA in the `hc` mode.
	ContrastPairsAAA [][2]string `yaml:"contrast_pairs_aaa"`
}

// Tree is a recursive map of string keys to either a string scalar or a
// nested Tree. yaml.v3 unmarshals into interface{}; we normalize to this
// shape post-parse so callers can walk it uniformly.
type Tree map[string]any

// Modes returns the list of semantic mode names ("dark", "light", "hc"),
// sorted with the canonical default first.
func (t Tokens) Modes() []string {
	out := make([]string, 0, len(t.Semantic))
	for k := range t.Semantic {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		return modeRank(out[i]) < modeRank(out[j])
	})
	return out
}

func modeRank(m string) int {
	switch m {
	case "dark":
		return 0
	case "light":
		return 1
	case "hc":
		return 2
	}
	return 100
}

var refPattern = regexp.MustCompile(`^\{([^{}]+)\}$`)

// Load reads, parses, and resolves a token source file.
// Every "{dotted.path}" reference inside the semantic section is replaced
// by the matching reference scalar; an unresolved reference is an error.
func Load(path string) (*Tokens, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var t Tokens
	if err := yaml.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if t.Reference == nil {
		return nil, fmt.Errorf("%s: missing reference section", path)
	}
	if len(t.Semantic) == 0 {
		return nil, fmt.Errorf("%s: missing semantic section", path)
	}
	for mode, tree := range t.Semantic {
		resolved, err := resolveTree(tree, t.Reference, mode)
		if err != nil {
			return nil, err
		}
		t.Semantic[mode] = resolved
	}
	return &t, nil
}

func resolveTree(node Tree, ref Tree, mode string) (Tree, error) {
	out := make(Tree, len(node))
	for k, v := range node {
		switch v := v.(type) {
		case string:
			r, err := resolveScalar(v, ref)
			if err != nil {
				return nil, fmt.Errorf("semantic.%s.%s: %w", mode, k, err)
			}
			out[k] = r
		case map[string]any:
			child, err := resolveTree(Tree(v), ref, mode+"."+k)
			if err != nil {
				return nil, err
			}
			out[k] = child
		case Tree:
			child, err := resolveTree(v, ref, mode+"."+k)
			if err != nil {
				return nil, err
			}
			out[k] = child
		default:
			return nil, fmt.Errorf("semantic.%s.%s: unsupported value type %T", mode, k, v)
		}
	}
	return out, nil
}

func resolveScalar(v string, ref Tree) (string, error) {
	m := refPattern.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return v, nil
	}
	target, ok := lookup(ref, strings.Split(m[1], "."))
	if !ok {
		return "", fmt.Errorf("unresolved reference {%s}", m[1])
	}
	s, ok := target.(string)
	if !ok {
		return "", fmt.Errorf("reference {%s} resolves to non-scalar %T", m[1], target)
	}
	return s, nil
}

func lookup(t Tree, path []string) (any, bool) {
	if len(path) == 0 {
		return nil, false
	}
	cur := any(t)
	for _, seg := range path {
		switch m := cur.(type) {
		case Tree:
			v, ok := m[seg]
			if !ok {
				return nil, false
			}
			cur = v
		case map[string]any:
			v, ok := m[seg]
			if !ok {
				return nil, false
			}
			cur = v
		default:
			return nil, false
		}
	}
	return cur, true
}

// Walk visits every leaf scalar in the tree, calling fn with the dotted path
// and value. Iteration order is deterministic (alphabetical at each level).
func Walk(t Tree, fn func(path []string, value string)) {
	walk(t, nil, fn)
}

func walk(node any, prefix []string, fn func(path []string, value string)) {
	switch m := node.(type) {
	case Tree:
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			walk(m[k], append(append([]string{}, prefix...), k), fn)
		}
	case map[string]any:
		walk(Tree(m), prefix, fn)
	case string:
		fn(prefix, m)
	}
}
