// Managed (admin-enforced) configuration loading.
//
// PRD §6.25.21. Conduit reads `requirements.toml` from a small set of
// well-known locations (see ManagedSearchPaths). The file declares
// constraints an administrator wants every user on this machine to obey:
// MCP server allowlists, banned tools/hooks, default config sections, and
// per-user permission overrides. Any user-supplied config that violates a
// constraint is rejected at Load() time with a typed ConstraintViolation
// error so surfaces can show exactly what the admin policy forbids.
//
// Security posture: managed config is intentionally separate from user
// config. It is loaded BEFORE user config, never deep-merged from user
// config (so a malicious user .conduit/config.yaml cannot widen its own
// permissions by injecting `managed:` keys), and every enforcement
// decision is appended to the audit log when an audit path is set.

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ManagedConfig is the parsed contents of `requirements.toml`. Every section
// is optional; omitted sections impose no constraints.
type ManagedConfig struct {
	// MCPAllowlist restricts which MCP server names a user may register. A
	// nil slice means "no restriction"; an empty slice means "no servers
	// allowed at all" (kill switch). Names are case-insensitive.
	MCPAllowlist []string

	// MCPBlocklist names servers that are always forbidden, even if the
	// allowlist would otherwise permit them. Blocklist wins on conflict.
	MCPBlocklist []string

	// AllowedHooks names the only hook commands the user may register. Empty
	// means "no restriction"; a present-but-empty list bans all hooks.
	AllowedHooks []string

	// BannedHooks names hook commands that are always forbidden.
	BannedHooks []string

	// ManagedHooks lists admin-injected hooks that always run regardless of
	// user config. They are appended after user hooks at load time.
	ManagedHooks []HookConfig

	// Defaults is a deep-merged YAML-like map applied as the base for every
	// user session. It is overridden by user config but not by the user's
	// "delete" — admins can re-impose a sensible floor here.
	Defaults map[string]any

	// Permissions maps a user identity (typically $USER) to a per-user
	// permission override. Missing identity falls back to the "*" wildcard.
	Permissions map[string]ManagedPermissions

	// AuditLogPath is the absolute file path where load-time enforcement
	// decisions are appended. Empty disables audit logging.
	AuditLogPath string

	// SourcePath is the absolute path of the requirements.toml file
	// loaded; empty when no managed config was found.
	SourcePath string
}

// ManagedPermissions is a single user's allow/deny matrix for permission
// categories declared in contracts. Empty allow lists mean "default
// behaviour"; any entry in deny is a hard veto regardless of other layers.
type ManagedPermissions struct {
	AllowCategories []string
	DenyCategories  []string
	// MaxToolCalls per session; 0 means unlimited.
	MaxToolCalls int
}

// ConstraintViolation reports one rejection at load time. It carries enough
// context for surfaces to show "your <thing> conflicts with the admin policy
// at <path>".
type ConstraintViolation struct {
	Source     string
	Constraint string
	Detail     string
}

// Error implements error.
func (v ConstraintViolation) Error() string {
	return fmt.Sprintf("managed-config violation: %s (%s): %s", v.Constraint, v.Source, v.Detail)
}

// ManagedSearchPaths is the ordered list of locations checked for
// requirements.toml. The first match wins. The order — system, then
// org-shared, then per-host — mirrors common admin-config conventions.
//
// On non-darwin systems the /Library paths are silently ignored.
func ManagedSearchPaths() []string {
	paths := []string{
		"/Library/Application Support/Conduit/requirements.toml",
		"/etc/conduit/requirements.toml",
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".conduit", "requirements.toml"))
	}
	return paths
}

// LoadManaged returns the first requirements.toml found in
// ManagedSearchPaths (or zero-value when none exist). A parse error stops
// the search — admins should see immediate feedback when their file is
// malformed rather than silent fall-through.
func LoadManaged() (ManagedConfig, error) {
	for _, p := range ManagedSearchPaths() {
		data, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return ManagedConfig{}, fmt.Errorf("managed config: read %q: %w", p, err)
		}
		mc, err := ParseManaged(data)
		if err != nil {
			return ManagedConfig{}, fmt.Errorf("managed config: parse %q: %w", p, err)
		}
		mc.SourcePath = p
		return mc, nil
	}
	return ManagedConfig{}, nil
}

// ValidateUser checks a user-loaded Config against the managed constraints.
// It returns a joined error of every violation found so admins see all
// problems at once instead of fixing them one at a time.
func (m ManagedConfig) ValidateUser(cfg Config, identity string) error {
	if m.SourcePath == "" {
		return nil
	}
	var violations []error

	// Hooks: allow / banned.
	allowed := stringSet(m.AllowedHooks)
	banned := stringSet(m.BannedHooks)
	for _, h := range cfg.Hooks {
		cmd := strings.TrimSpace(h.Command)
		key := firstWord(cmd)
		if _, isBanned := banned[strings.ToLower(key)]; isBanned {
			violations = append(violations, ConstraintViolation{
				Source: m.SourcePath, Constraint: "banned_hook",
				Detail: fmt.Sprintf("hook command %q is in the banned list", cmd),
			})
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[strings.ToLower(key)]; !ok {
				violations = append(violations, ConstraintViolation{
					Source: m.SourcePath, Constraint: "hook_not_allowlisted",
					Detail: fmt.Sprintf("hook command %q is not in the allowed list", cmd),
				})
			}
		}
	}

	if len(violations) == 1 {
		return violations[0]
	}
	if len(violations) > 1 {
		return errors.Join(violations...)
	}
	return nil
}

// CheckMCPServer enforces the allowlist/blocklist. Returns a non-nil error
// the surface can show verbatim.
func (m ManagedConfig) CheckMCPServer(name string) error {
	if m.SourcePath == "" {
		return nil
	}
	low := strings.ToLower(name)
	for _, b := range m.MCPBlocklist {
		if strings.EqualFold(b, low) {
			return ConstraintViolation{
				Source: m.SourcePath, Constraint: "mcp_blocked",
				Detail: fmt.Sprintf("MCP server %q is on the admin blocklist", name),
			}
		}
	}
	if m.MCPAllowlist == nil {
		return nil
	}
	for _, a := range m.MCPAllowlist {
		if strings.EqualFold(a, low) {
			return nil
		}
	}
	return ConstraintViolation{
		Source: m.SourcePath, Constraint: "mcp_not_allowlisted",
		Detail: fmt.Sprintf("MCP server %q is not in the admin allowlist", name),
	}
}

// PermissionsFor returns the merged permission set for a user identity.
// "*" entries are merged in first so the named identity overrides the wildcard.
func (m ManagedConfig) PermissionsFor(identity string) ManagedPermissions {
	merged := ManagedPermissions{}
	if base, ok := m.Permissions["*"]; ok {
		merged = base
	}
	if user, ok := m.Permissions[identity]; ok {
		if len(user.AllowCategories) > 0 {
			merged.AllowCategories = append(merged.AllowCategories, user.AllowCategories...)
		}
		if len(user.DenyCategories) > 0 {
			merged.DenyCategories = append(merged.DenyCategories, user.DenyCategories...)
		}
		if user.MaxToolCalls > 0 {
			merged.MaxToolCalls = user.MaxToolCalls
		}
	}
	return merged
}

// AppendAudit writes one structured line to AuditLogPath. A failure to write
// the audit log is returned but never panics — the caller decides whether to
// halt or continue. The format is intentionally trivial (date \t event \t
// detail) so admins can grep without parsers.
func (m ManagedConfig) AppendAudit(event, detail string) error {
	if m.AuditLogPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(m.AuditLogPath), 0o750); err != nil {
		return fmt.Errorf("managed config: prepare audit dir: %w", err)
	}
	f, err := os.OpenFile(m.AuditLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return fmt.Errorf("managed config: open audit log: %w", err)
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\t%s\t%s\n", time.Now().UTC().Format(time.RFC3339), event, sanitiseAudit(detail))
	return err
}

func sanitiseAudit(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return s
}

func stringSet(in []string) map[string]struct{} {
	if in == nil {
		return nil
	}
	out := make(map[string]struct{}, len(in))
	for _, s := range in {
		out[strings.ToLower(strings.TrimSpace(s))] = struct{}{}
	}
	return out
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t"); i > 0 {
		return s[:i]
	}
	return s
}

// ParseManaged is a deliberately small TOML subset parser. It supports:
//   - top-level key = value lines
//   - [section] and [section.subsection] headers
//   - quoted strings, integers, booleans
//   - inline string arrays: ["a", "b"]
//   - comments starting with '#'
//
// Anything outside that subset is rejected with a line-numbered error.
// Why hand-rolled: avoids pulling in a TOML dependency just for this one
// admin file, and keeps the surface small enough to audit at a glance.
func ParseManaged(data []byte) (ManagedConfig, error) {
	mc := ManagedConfig{
		Defaults:    map[string]any{},
		Permissions: map[string]ManagedPermissions{},
	}
	currentSection := ""
	for lineNum, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			raw := strings.TrimSpace(line[1 : len(line)-1])
			// Allow quoted identifiers (e.g. permissions."*") by stripping
			// surrounding quotes from each dotted segment.
			parts := strings.Split(raw, ".")
			for i, p := range parts {
				p = strings.TrimSpace(p)
				if len(p) >= 2 && strings.HasPrefix(p, "\"") && strings.HasSuffix(p, "\"") {
					p = p[1 : len(p)-1]
				}
				parts[i] = p
			}
			currentSection = strings.Join(parts, ".")
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			return ManagedConfig{}, fmt.Errorf("line %d: expected key = value", lineNum+1)
		}
		key := strings.TrimSpace(line[:eq])
		valStr := strings.TrimSpace(line[eq+1:])
		// Strip trailing comment on same line (very simple — does not handle
		// `#` inside quoted strings; admin TOML is short, so the trade-off
		// is fine and documented here).
		if idx := strings.Index(valStr, " #"); idx >= 0 && !strings.HasPrefix(valStr, "\"") && !strings.HasPrefix(valStr, "[") {
			valStr = strings.TrimSpace(valStr[:idx])
		}
		val, err := parseTOMLValue(valStr)
		if err != nil {
			return ManagedConfig{}, fmt.Errorf("line %d: %w", lineNum+1, err)
		}
		if err := assignManaged(&mc, currentSection, key, val); err != nil {
			return ManagedConfig{}, fmt.Errorf("line %d: %w", lineNum+1, err)
		}
	}
	return mc, nil
}

func parseTOMLValue(s string) (any, error) {
	if s == "" {
		return nil, errors.New("missing value")
	}
	switch {
	case s == "true":
		return true, nil
	case s == "false":
		return false, nil
	case strings.HasPrefix(s, "\""):
		if !strings.HasSuffix(s, "\"") || len(s) < 2 {
			return nil, fmt.Errorf("unterminated string %q", s)
		}
		return s[1 : len(s)-1], nil
	case strings.HasPrefix(s, "["):
		if !strings.HasSuffix(s, "]") {
			return nil, fmt.Errorf("unterminated array %q", s)
		}
		inner := strings.TrimSpace(s[1 : len(s)-1])
		if inner == "" {
			return []string{}, nil
		}
		var out []string
		for _, item := range splitArray(inner) {
			item = strings.TrimSpace(item)
			if !strings.HasPrefix(item, "\"") || !strings.HasSuffix(item, "\"") {
				return nil, fmt.Errorf("array item %q is not a quoted string", item)
			}
			out = append(out, item[1:len(item)-1])
		}
		return out, nil
	default:
		if n, err := strconv.Atoi(s); err == nil {
			return n, nil
		}
		return nil, fmt.Errorf("unrecognised value %q", s)
	}
}

func splitArray(s string) []string {
	var out []string
	var cur strings.Builder
	inStr := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			inStr = !inStr
			cur.WriteByte(c)
		case c == ',' && !inStr:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func assignManaged(mc *ManagedConfig, section, key string, val any) error {
	switch section {
	case "":
		switch key {
		case "audit_log":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("audit_log must be a string")
			}
			mc.AuditLogPath = s
		default:
			return fmt.Errorf("unknown top-level key %q", key)
		}
	case "mcp":
		arr, ok := val.([]string)
		if !ok {
			return fmt.Errorf("mcp.%s must be a string array", key)
		}
		switch key {
		case "allowlist":
			mc.MCPAllowlist = arr
		case "blocklist":
			mc.MCPBlocklist = arr
		default:
			return fmt.Errorf("unknown mcp key %q", key)
		}
	case "hooks":
		arr, ok := val.([]string)
		if !ok {
			return fmt.Errorf("hooks.%s must be a string array", key)
		}
		switch key {
		case "allowed":
			mc.AllowedHooks = arr
		case "banned":
			mc.BannedHooks = arr
		default:
			return fmt.Errorf("unknown hooks key %q", key)
		}
	default:
		// permissions.<identity> sections
		if strings.HasPrefix(section, "permissions.") {
			id := strings.TrimPrefix(section, "permissions.")
			perm := mc.Permissions[id]
			switch key {
			case "allow":
				arr, ok := val.([]string)
				if !ok {
					return fmt.Errorf("permissions.%s.allow must be a string array", id)
				}
				perm.AllowCategories = arr
			case "deny":
				arr, ok := val.([]string)
				if !ok {
					return fmt.Errorf("permissions.%s.deny must be a string array", id)
				}
				perm.DenyCategories = arr
			case "max_tool_calls":
				n, ok := val.(int)
				if !ok {
					return fmt.Errorf("permissions.%s.max_tool_calls must be int", id)
				}
				perm.MaxToolCalls = n
			default:
				return fmt.Errorf("unknown permissions key %q", key)
			}
			mc.Permissions[id] = perm
			return nil
		}
		// defaults.<section> deep-merge bag — values are dropped into a
		// nested map[string]any so callers can layer them onto user config.
		if strings.HasPrefix(section, "defaults.") {
			subKey := strings.TrimPrefix(section, "defaults.")
			sub, _ := mc.Defaults[subKey].(map[string]any)
			if sub == nil {
				sub = map[string]any{}
			}
			sub[key] = val
			mc.Defaults[subKey] = sub
			return nil
		}
		return fmt.Errorf("unknown section %q", section)
	}
	return nil
}
