package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleTOML = `
audit_log = "/tmp/conduit-audit.log"

[mcp]
allowlist = ["github", "linear"]
blocklist = ["evil-server"]

[hooks]
allowed = ["log-tool", "scan-prompt"]
banned = ["curl"]

[permissions."*"]
deny = ["shell"]
max_tool_calls = 100

[permissions.alice]
allow = ["shell"]
max_tool_calls = 500

[defaults.web_search]
mode = "cached"
`

func TestParseManagedSubset(t *testing.T) {
	mc, err := ParseManaged([]byte(sampleTOML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if mc.AuditLogPath != "/tmp/conduit-audit.log" {
		t.Errorf("audit log: %q", mc.AuditLogPath)
	}
	if len(mc.MCPAllowlist) != 2 || mc.MCPAllowlist[0] != "github" {
		t.Errorf("mcp allowlist: %+v", mc.MCPAllowlist)
	}
	if len(mc.MCPBlocklist) != 1 || mc.MCPBlocklist[0] != "evil-server" {
		t.Errorf("blocklist: %+v", mc.MCPBlocklist)
	}
	if len(mc.AllowedHooks) != 2 {
		t.Errorf("hooks allowed: %+v", mc.AllowedHooks)
	}
	if mc.Permissions["*"].MaxToolCalls != 100 {
		t.Errorf("wildcard max: %d", mc.Permissions["*"].MaxToolCalls)
	}
	if mc.Permissions["alice"].AllowCategories[0] != "shell" {
		t.Errorf("alice allow: %+v", mc.Permissions["alice"])
	}
	web, _ := mc.Defaults["web_search"].(map[string]any)
	if web["mode"] != "cached" {
		t.Errorf("defaults: %+v", mc.Defaults)
	}
}

func TestParseManagedRejectsUnknownKey(t *testing.T) {
	_, err := ParseManaged([]byte("[mcp]\nbogus = [\"x\"]\n"))
	if err == nil || !strings.Contains(err.Error(), "unknown mcp key") {
		t.Fatalf("expected unknown-key error, got %v", err)
	}
}

func TestParseManagedRejectsMalformed(t *testing.T) {
	cases := []string{
		"key without value",
		"[unclosed",
		"k = \"unterminated",
	}
	for _, c := range cases {
		if _, err := ParseManaged([]byte(c)); err == nil {
			t.Errorf("input %q should fail", c)
		}
	}
}

func TestCheckMCPServerAllowAndBlock(t *testing.T) {
	mc := ManagedConfig{
		SourcePath:   "/etc/conduit/requirements.toml",
		MCPAllowlist: []string{"github"},
		MCPBlocklist: []string{"evil"},
	}
	if err := mc.CheckMCPServer("github"); err != nil {
		t.Errorf("github should be allowed: %v", err)
	}
	if err := mc.CheckMCPServer("linear"); err == nil {
		t.Error("linear should be rejected (not allowlisted)")
	}
	if err := mc.CheckMCPServer("evil"); err == nil {
		t.Error("evil should be blocked")
	}
	// Empty allowlist still allows blocklist enforcement.
	mc2 := ManagedConfig{SourcePath: "x", MCPBlocklist: []string{"bad"}}
	if err := mc2.CheckMCPServer("good"); err != nil {
		t.Errorf("nil allowlist should permit good: %v", err)
	}
	if err := mc2.CheckMCPServer("bad"); err == nil {
		t.Error("blocklist should win even without allowlist")
	}
}

func TestCheckMCPServerNoSourceIsNoOp(t *testing.T) {
	mc := ManagedConfig{}
	if err := mc.CheckMCPServer("anything"); err != nil {
		t.Errorf("no source should be no-op: %v", err)
	}
}

func TestValidateUserHookConstraints(t *testing.T) {
	mc := ManagedConfig{
		SourcePath:   "/etc/conduit/requirements.toml",
		AllowedHooks: []string{"log-tool"},
		BannedHooks:  []string{"curl"},
	}
	cfg := Config{Hooks: []HookConfig{
		{Event: "PreToolUse", Command: "log-tool --json"},
		{Event: "PreToolUse", Command: "curl https://example.com"},
		{Event: "PreToolUse", Command: "rogue-hook"},
	}}
	err := mc.ValidateUser(cfg, "alice")
	if err == nil {
		t.Fatal("expected violations")
	}
	msg := err.Error()
	if !strings.Contains(msg, "banned_hook") || !strings.Contains(msg, "hook_not_allowlisted") {
		t.Fatalf("missing expected violations in %q", msg)
	}
}

func TestPermissionsForMergesWildcardAndUser(t *testing.T) {
	mc := ManagedConfig{Permissions: map[string]ManagedPermissions{
		"*":     {DenyCategories: []string{"shell"}, MaxToolCalls: 100},
		"alice": {AllowCategories: []string{"shell"}, MaxToolCalls: 500},
	}}
	p := mc.PermissionsFor("alice")
	if p.MaxToolCalls != 500 {
		t.Errorf("max: %d", p.MaxToolCalls)
	}
	if len(p.AllowCategories) != 1 || p.AllowCategories[0] != "shell" {
		t.Errorf("allow: %+v", p.AllowCategories)
	}
	if len(p.DenyCategories) != 1 {
		t.Errorf("deny should be inherited from wildcard: %+v", p.DenyCategories)
	}
	bob := mc.PermissionsFor("bob")
	if bob.MaxToolCalls != 100 {
		t.Errorf("bob should inherit wildcard: %+v", bob)
	}
}

func TestAppendAuditWritesLine(t *testing.T) {
	dir := t.TempDir()
	mc := ManagedConfig{AuditLogPath: filepath.Join(dir, "audit.log")}
	if err := mc.AppendAudit("mcp_blocked", "evil-server\nattempt"); err != nil {
		t.Fatalf("append: %v", err)
	}
	data, err := os.ReadFile(mc.AuditLogPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "mcp_blocked") {
		t.Fatalf("audit missing event: %s", data)
	}
	if strings.Contains(string(data), "\nattempt") {
		t.Fatal("newline in detail should be sanitised")
	}
}

func TestConstraintViolationIsTyped(t *testing.T) {
	mc := ManagedConfig{SourcePath: "/x", MCPBlocklist: []string{"bad"}}
	err := mc.CheckMCPServer("bad")
	var v ConstraintViolation
	if !errors.As(err, &v) {
		t.Fatalf("expected ConstraintViolation, got %T", err)
	}
	if v.Constraint != "mcp_blocked" {
		t.Errorf("constraint: %q", v.Constraint)
	}
}
