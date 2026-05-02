package workflow

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// ErrEmptyDocument is returned by Parse when the input contains no YAML
// nodes (e.g. an empty file or a file containing only comments).
var ErrEmptyDocument = errors.New("workflow: empty document")

// Parse decodes a YAML workflow document into a WorkflowDefinition.
//
// Unknown fields are rejected (yaml.Decoder.KnownFields(true)) so authors
// catch typos at parse time. Parse does not validate semantic constraints;
// callers should follow with Validate.
func Parse(data []byte) (contracts.WorkflowDefinition, error) {
	var def contracts.WorkflowDefinition
	if len(bytes.TrimSpace(data)) == 0 {
		return def, ErrEmptyDocument
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&def); err != nil {
		return def, fmt.Errorf("workflow: parse: %w", err)
	}
	return def, nil
}

// ParseFile reads path and decodes its contents via Parse.
func ParseFile(path string) (contracts.WorkflowDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return contracts.WorkflowDefinition{}, fmt.Errorf("workflow: read %q: %w", path, err)
	}
	return Parse(data)
}
