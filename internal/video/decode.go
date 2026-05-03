package video

import (
	"encoding/json"
)

// decodeOps unmarshals a JSON array of Op-shaped objects into []Op.
// Lives in its own file so the rest of the package keeps zero non-stdlib
// imports beyond encoding/json.
func decodeOps(s string) ([]Op, error) {
	var raw []struct {
		Kind   string         `json:"kind"`
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, err
	}
	ops := make([]Op, len(raw))
	for i, r := range raw {
		ops[i] = Op{Kind: r.Kind, Params: r.Params}
	}
	return ops, nil
}
