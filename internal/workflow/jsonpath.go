package workflow

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// jsonPath evaluates a minimal JSONPath subset against root and returns
// the resolved value. The supported grammar is intentionally narrow
// (PRD §6.7 only requires simple field/index access):
//
//	$                            -> the root
//	$.field                      -> object field
//	$.a.b.c                      -> nested field access
//	$.arr[0]                     -> array index (non-negative integer)
//	$.arr[0].field               -> mixed access
//	$["weird key"]               -> bracket field access (single or double quoted)
//
// Wildcards, recursive descent, filters, and slices are not supported;
// callers needing them can compose multiple Conditions or extend this
// evaluator. The second return value is false when the path cannot be
// resolved against root (missing field, out-of-range index, type
// mismatch); callers should treat that as a non-match rather than an
// error so that conditions remain composable.
func jsonPath(expr string, root any) (any, bool, error) {
	if expr == "" {
		return nil, false, errors.New("workflow: empty jsonpath")
	}
	if expr[0] != '$' {
		return nil, false, fmt.Errorf("workflow: jsonpath must start with '$': %q", expr)
	}
	tokens, err := tokenizePath(expr[1:])
	if err != nil {
		return nil, false, err
	}
	cur := root
	for _, tok := range tokens {
		next, ok := applyToken(cur, tok)
		if !ok {
			return nil, false, nil
		}
		cur = next
	}
	return cur, true, nil
}

// pathToken is one access step in a JSONPath expression. Exactly one of
// field or index is meaningful; isIndex selects which.
type pathToken struct {
	field   string
	index   int
	isIndex bool
}

// tokenizePath splits the JSONPath body (everything after the leading $)
// into a sequence of pathTokens.
func tokenizePath(body string) ([]pathToken, error) {
	var tokens []pathToken
	i := 0
	for i < len(body) {
		c := body[i]
		switch c {
		case '.':
			i++
			start := i
			for i < len(body) && body[i] != '.' && body[i] != '[' {
				i++
			}
			if start == i {
				return nil, fmt.Errorf("workflow: empty field name in jsonpath at %d", start)
			}
			tokens = append(tokens, pathToken{field: body[start:i]})
		case '[':
			end := strings.IndexByte(body[i:], ']')
			if end == -1 {
				return nil, fmt.Errorf("workflow: unclosed '[' in jsonpath at %d", i)
			}
			inner := body[i+1 : i+end]
			tok, err := parseBracket(inner)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			i += end + 1
		default:
			return nil, fmt.Errorf("workflow: unexpected %q in jsonpath at %d", c, i)
		}
	}
	return tokens, nil
}

// parseBracket parses the contents between '[' and ']'. Numeric content
// becomes an index token; quoted content becomes a field token.
func parseBracket(inner string) (pathToken, error) {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return pathToken{}, errors.New("workflow: empty bracket in jsonpath")
	}
	if (strings.HasPrefix(inner, "\"") && strings.HasSuffix(inner, "\"")) ||
		(strings.HasPrefix(inner, "'") && strings.HasSuffix(inner, "'")) {
		if len(inner) < 2 {
			return pathToken{}, errors.New("workflow: malformed quoted bracket in jsonpath")
		}
		return pathToken{field: inner[1 : len(inner)-1]}, nil
	}
	idx, err := strconv.Atoi(inner)
	if err != nil {
		return pathToken{}, fmt.Errorf("workflow: invalid bracket %q in jsonpath", inner)
	}
	if idx < 0 {
		return pathToken{}, fmt.Errorf("workflow: negative index %d in jsonpath", idx)
	}
	return pathToken{index: idx, isIndex: true}, nil
}

// applyToken descends one step into v. Objects are matched against
// map[string]any; arrays against []any. Returns false when the value
// cannot be reached.
func applyToken(v any, tok pathToken) (any, bool) {
	if v == nil {
		return nil, false
	}
	if tok.isIndex {
		switch arr := v.(type) {
		case []any:
			if tok.index >= len(arr) {
				return nil, false
			}
			return arr[tok.index], true
		case []string:
			if tok.index >= len(arr) {
				return nil, false
			}
			return arr[tok.index], true
		}
		return nil, false
	}
	if m, ok := v.(map[string]any); ok {
		val, present := m[tok.field]
		if !present {
			return nil, false
		}
		return val, true
	}
	if m, ok := v.(map[string]string); ok {
		val, present := m[tok.field]
		if !present {
			return nil, false
		}
		return val, true
	}
	return nil, false
}
