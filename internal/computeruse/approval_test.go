package computeruse_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/computeruse"
)

func newStore(t *testing.T) (*computeruse.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "approved-apps.json")
	store, err := computeruse.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return store, path
}

func TestStore_ApproveThenIsApproved(t *testing.T) {
	store, _ := newStore(t)
	app := computeruse.AppRef{BundleID: "com.apple.Safari", Name: "Safari"}

	if store.IsApproved(app) {
		t.Fatal("IsApproved should be false before Approve")
	}

	rec, err := store.Approve(app, computeruse.ScopeFull, "onboarding")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if rec.Scope != computeruse.ScopeFull {
		t.Errorf("Scope = %q, want %q", rec.Scope, computeruse.ScopeFull)
	}
	if rec.ApprovedAt.IsZero() {
		t.Error("ApprovedAt should be populated")
	}
	if !rec.Active() {
		t.Error("freshly-approved record should be Active()")
	}
	if !store.IsApproved(app) {
		t.Error("IsApproved should be true after Approve")
	}
}

func TestStore_DefaultScopeIsFull(t *testing.T) {
	store, _ := newStore(t)
	rec, err := store.Approve(computeruse.AppRef{BundleID: "com.example"}, "", "")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if rec.Scope != computeruse.ScopeFull {
		t.Errorf("default Scope = %q, want %q", rec.Scope, computeruse.ScopeFull)
	}
}

func TestStore_RejectsEmptyAppRef(t *testing.T) {
	store, _ := newStore(t)
	if _, err := store.Approve(computeruse.AppRef{}, computeruse.ScopeFull, ""); !errors.Is(err, computeruse.ErrEmptyApp) {
		t.Errorf("Approve(empty) err = %v, want ErrEmptyApp", err)
	}
	if store.IsApproved(computeruse.AppRef{}) {
		t.Error("IsApproved(empty) should be false")
	}
}

func TestStore_RoundTripPersistsApprovals(t *testing.T) {
	store, path := newStore(t)
	app := computeruse.AppRef{BundleID: "com.apple.Mail", Name: "Mail"}
	if _, err := store.Approve(app, computeruse.ScopeReadOnly, "audit"); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// Re-open from disk and verify the record is preserved verbatim.
	reopened, err := computeruse.Open(path)
	if err != nil {
		t.Fatalf("Open after Approve: %v", err)
	}
	if !reopened.IsApproved(app) {
		t.Error("approval should survive reopen")
	}
	rec, ok := reopened.Lookup(app)
	if !ok {
		t.Fatal("Lookup should find the persisted record")
	}
	if rec.Scope != computeruse.ScopeReadOnly {
		t.Errorf("persisted Scope = %q, want %q", rec.Scope, computeruse.ScopeReadOnly)
	}
	if rec.Note != "audit" {
		t.Errorf("persisted Note = %q, want %q", rec.Note, "audit")
	}
}

func TestStore_Revoke(t *testing.T) {
	store, path := newStore(t)
	app := computeruse.AppRef{BundleID: "com.apple.Terminal"}
	if _, err := store.Approve(app, computeruse.ScopeFull, ""); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	rec, err := store.Revoke(app)
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if rec.RevokedAt == nil || rec.RevokedAt.IsZero() {
		t.Error("RevokedAt should be populated after Revoke")
	}
	if rec.Active() {
		t.Error("revoked record should not be Active()")
	}
	if store.IsApproved(app) {
		t.Error("IsApproved should be false after Revoke")
	}

	// Revocation persists across reopen and is preserved (audit trail).
	reopened, err := computeruse.Open(path)
	if err != nil {
		t.Fatalf("Open after Revoke: %v", err)
	}
	if reopened.IsApproved(app) {
		t.Error("revoked status should survive reopen")
	}
	if got, ok := reopened.Lookup(app); !ok || got.Active() {
		t.Errorf("Lookup after reopen: rec=%+v ok=%v want revoked record", got, ok)
	}
}

func TestStore_RevokeUnknownReturnsSentinel(t *testing.T) {
	store, _ := newStore(t)
	if _, err := store.Revoke(computeruse.AppRef{BundleID: "com.unknown"}); !errors.Is(err, computeruse.ErrUnknownApp) {
		t.Errorf("Revoke(unknown) err = %v, want ErrUnknownApp", err)
	}
}

func TestStore_ApproveAfterRevokeReinstates(t *testing.T) {
	store, _ := newStore(t)
	app := computeruse.AppRef{BundleID: "com.apple.Safari"}

	if _, err := store.Approve(app, computeruse.ScopeFull, ""); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if _, err := store.Revoke(app); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := store.Approve(app, computeruse.ScopeFull, "re-approved"); err != nil {
		t.Fatalf("re-Approve: %v", err)
	}
	if !store.IsApproved(app) {
		t.Error("re-approved app should be approved again")
	}
	rec, _ := store.Lookup(app)
	if !rec.Active() {
		t.Error("re-approved record should be Active()")
	}
	if rec.RevokedAt != nil {
		t.Error("re-approval should clear RevokedAt")
	}
}

func TestStore_List(t *testing.T) {
	store, _ := newStore(t)
	apps := []computeruse.AppRef{
		{BundleID: "com.apple.Safari"},
		{BundleID: "com.apple.Mail"},
		{Name: "OldUtility"},
	}
	for _, a := range apps {
		if _, err := store.Approve(a, computeruse.ScopeFull, ""); err != nil {
			t.Fatalf("Approve %v: %v", a, err)
		}
	}
	if _, err := store.Revoke(apps[2]); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	if got := store.List(); len(got) != 3 {
		t.Errorf("List len = %d, want 3 (revoked rows kept for audit)", len(got))
	}
	if got := store.ActiveApprovals(); len(got) != 2 {
		t.Errorf("ActiveApprovals len = %d, want 2", len(got))
	}
}

func TestStore_RequestApprovalUsesConfirm(t *testing.T) {
	store, _ := newStore(t)
	app := computeruse.AppRef{BundleID: "com.example.app"}

	called := 0
	confirm := func(a computeruse.AppRef) (computeruse.Scope, string, error) {
		called++
		if a.BundleID != app.BundleID {
			t.Errorf("confirm got app %v, want %v", a, app)
		}
		return computeruse.ScopeReadOnly, "user said ok", nil
	}

	rec, err := store.RequestApproval(app, confirm)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if rec.Scope != computeruse.ScopeReadOnly {
		t.Errorf("Scope = %q, want %q", rec.Scope, computeruse.ScopeReadOnly)
	}
	if !store.IsApproved(app) {
		t.Error("IsApproved should be true after RequestApproval")
	}

	// A second RequestApproval for the same active app must NOT prompt again.
	if _, err := store.RequestApproval(app, confirm); err != nil {
		t.Fatalf("RequestApproval (already approved): %v", err)
	}
	if called != 1 {
		t.Errorf("confirm called %d times, want 1 (cached)", called)
	}
}

func TestStore_RequestApprovalNilConfirmDeniesByDefault(t *testing.T) {
	store, _ := newStore(t)
	app := computeruse.AppRef{BundleID: "com.example"}
	if _, err := store.RequestApproval(app, nil); !errors.Is(err, computeruse.ErrNotApproved) {
		t.Errorf("RequestApproval(nil) err = %v, want ErrNotApproved", err)
	}
	if store.IsApproved(app) {
		t.Error("denied request should not produce an approval")
	}
}

func TestStore_EnsureApprovedReturnsSentinel(t *testing.T) {
	store, _ := newStore(t)
	app := computeruse.AppRef{BundleID: "com.example"}

	err := store.EnsureApproved(app)
	if !errors.Is(err, computeruse.ErrNotApproved) {
		t.Fatalf("EnsureApproved err = %v, want ErrNotApproved", err)
	}
	if !strings.Contains(err.Error(), "com.example") {
		t.Errorf("EnsureApproved error %q should mention the app id", err)
	}

	if _, err := store.Approve(app, computeruse.ScopeFull, ""); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := store.EnsureApproved(app); err != nil {
		t.Errorf("EnsureApproved after Approve: %v", err)
	}
}

func TestStore_KeyMatchesByBundleIDFirst(t *testing.T) {
	store, _ := newStore(t)
	a := computeruse.AppRef{BundleID: "com.apple.Safari", Name: "Safari"}
	b := computeruse.AppRef{BundleID: "com.apple.Safari", Name: "AnotherSafariFork"}

	if _, err := store.Approve(a, computeruse.ScopeFull, ""); err != nil {
		t.Fatalf("Approve a: %v", err)
	}
	// Same bundle id with a different name must hit the same record.
	if !store.IsApproved(b) {
		t.Error("IsApproved should match by BundleID when names differ")
	}
}

func TestStore_KeyFallsBackToNameWhenBundleIDMissing(t *testing.T) {
	store, _ := newStore(t)
	a := computeruse.AppRef{Name: "MyTool"}
	b := computeruse.AppRef{Name: "  mytool  "} // case + whitespace

	if _, err := store.Approve(a, computeruse.ScopeFull, ""); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if !store.IsApproved(b) {
		t.Error("IsApproved should normalise name (case + whitespace)")
	}
}

func TestStore_OpenMissingFileIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope", "approved-apps.json")
	store, err := computeruse.Open(path)
	if err != nil {
		t.Fatalf("Open(missing): %v", err)
	}
	if got := store.List(); len(got) != 0 {
		t.Errorf("fresh store List() len = %d, want 0", len(got))
	}
}

func TestStore_OpenRejectsCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "approved-apps.json")
	if err := os.WriteFile(path, []byte("{ this is not json"), 0o644); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}
	if _, err := computeruse.Open(path); err == nil {
		t.Error("Open should fail on corrupt JSON")
	}
}

func TestStore_FileFormatIsHumanReadable(t *testing.T) {
	store, path := newStore(t)
	app := computeruse.AppRef{BundleID: "com.apple.Safari", Name: "Safari"}
	if _, err := store.Approve(app, computeruse.ScopeFull, "onboarding"); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	// Indented JSON keeps the file user-editable, matching the convention
	// established by the credentials/usage stores.
	if !strings.Contains(string(data), "  ") {
		t.Error("expected indented JSON output for human readability")
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("file is not valid JSON: %v", err)
	}
	if _, ok := parsed["records"]; !ok {
		t.Error("file should contain a records array")
	}
	if v, ok := parsed["version"].(float64); !ok || int(v) != 1 {
		t.Errorf("file version = %v, want 1", parsed["version"])
	}
}

func TestStore_AtomicSavePreservesPreviousFileOnError(t *testing.T) {
	// Verifies that a save() going through tmp+rename never leaves a
	// partially-written file. We can't easily inject a write failure, but we
	// can at least confirm that no .tmp-* siblings are left around after a
	// successful Approve cycle.
	store, path := newStore(t)
	for i := range [5]struct{}{} {
		app := computeruse.AppRef{BundleID: "com.example.app" + string(rune('A'+i))}
		if _, err := store.Approve(app, computeruse.ScopeFull, ""); err != nil {
			t.Fatalf("Approve %d: %v", i, err)
		}
	}
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestStore_ModifiedTimestampUpdatedOnSave(t *testing.T) {
	store, path := newStore(t)
	app := computeruse.AppRef{BundleID: "com.example"}
	if _, err := store.Approve(app, computeruse.ScopeFull, ""); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var parsed struct {
		Modified time.Time `json:"modified"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if parsed.Modified.IsZero() {
		t.Error("modified timestamp should be set after save")
	}
}
