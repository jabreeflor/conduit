package computeruse

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultApprovalsPath is the on-disk location for the persistent approval
// store. Mirrors the convention used by the usage tracker (#15) and credential
// pool (#14): a per-user file under ~/.conduit/.
const DefaultApprovalsPath = ".conduit/approved-apps.json"

// Scope narrows what an approval grants. Values are intentionally
// stringly-typed so users can hand-edit approved-apps.json without breaking
// older Conduit binaries.
type Scope string

const (
	// ScopeFull lets the agent perform any computer-use action against the app.
	ScopeFull Scope = "full"
	// ScopeReadOnly limits the agent to read-only inspection (screenshots,
	// accessibility queries) without sending input events.
	ScopeReadOnly Scope = "read_only"
)

// AppRef identifies a target macOS application. Either BundleID or Name must
// be set; BundleID is preferred because it is stable across renames.
type AppRef struct {
	BundleID string `json:"bundle_id,omitempty"`
	Name     string `json:"name,omitempty"`
}

// Key returns a stable lookup key for the app. Bundle IDs win when present;
// otherwise we fall back to a normalised app name.
func (a AppRef) Key() string {
	if id := strings.TrimSpace(a.BundleID); id != "" {
		return "bundle:" + strings.ToLower(id)
	}
	return "name:" + strings.ToLower(strings.TrimSpace(a.Name))
}

// IsZero reports whether the AppRef is empty (no bundle ID and no name).
func (a AppRef) IsZero() bool {
	return strings.TrimSpace(a.BundleID) == "" && strings.TrimSpace(a.Name) == ""
}

// ApprovalRecord is one row in the persistent approval store.
//
// Wire format (JSON):
//
//	{
//	  "app":         { "bundle_id": "com.apple.Safari", "name": "Safari" },
//	  "approved_at": "2026-05-02T12:00:00Z",
//	  "scope":       "full",
//	  "revoked_at":  null,
//	  "note":        "approved during onboarding"
//	}
type ApprovalRecord struct {
	App        AppRef     `json:"app"`
	ApprovedAt time.Time  `json:"approved_at"`
	Scope      Scope      `json:"scope"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	Note       string     `json:"note,omitempty"`
}

// Active reports whether the record currently grants access. Revoked records
// remain in the file so users keep an audit trail.
func (r ApprovalRecord) Active() bool {
	return r.RevokedAt == nil || r.RevokedAt.IsZero()
}

// Errors returned by the store. They are all sentinel values so callers can
// match with errors.Is.
var (
	// ErrNotApproved means the requested app has no active approval. This is
	// the error computer-use code paths must surface to the user.
	ErrNotApproved = errors.New("computeruse: app is not approved")
	// ErrUnknownApp is returned by Revoke when no record exists for the app.
	ErrUnknownApp = errors.New("computeruse: no approval found for app")
	// ErrEmptyApp guards against accidentally writing rows that match nothing.
	ErrEmptyApp = errors.New("computeruse: app must have a bundle_id or name")
)

// fileSchema is the on-disk wrapper around the records slice. Wrapping in a
// struct lets us add fields later without rewriting old files.
type fileSchema struct {
	Version  int              `json:"version"`
	Records  []ApprovalRecord `json:"records"`
	Modified time.Time        `json:"modified,omitempty"`
}

const currentSchemaVersion = 1

// Store is the persistent per-app approval store.
//
// All public methods are safe for concurrent use. Writes go through a tmp
// file + atomic rename so a crash mid-save can never leave a half-written
// approvals file.
type Store struct {
	path    string
	mu      sync.Mutex
	records map[string]ApprovalRecord
	now     func() time.Time
}

// Open loads (or initialises) the approval store at path. A missing file is
// not an error — first-run code paths simply start with an empty store.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("computeruse: store path is required")
	}
	s := &Store{
		path:    path,
		records: make(map[string]ApprovalRecord),
		now:     func() time.Time { return time.Now().UTC() },
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// OpenDefault opens the store at ~/.conduit/approved-apps.json.
func OpenDefault() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("computeruse: resolve home dir: %w", err)
	}
	return Open(filepath.Join(home, DefaultApprovalsPath))
}

// Path returns the on-disk location of this store.
func (s *Store) Path() string { return s.path }

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("computeruse: read %s: %w", s.path, err)
	}
	if len(data) == 0 {
		return nil
	}

	var file fileSchema
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("computeruse: parse %s: %w", s.path, err)
	}
	for _, rec := range file.Records {
		if rec.App.IsZero() {
			continue
		}
		s.records[rec.App.Key()] = rec
	}
	return nil
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("computeruse: create approvals dir: %w", err)
	}

	records := make([]ApprovalRecord, 0, len(s.records))
	for _, rec := range s.records {
		records = append(records, rec)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].App.Key() < records[j].App.Key()
	})

	file := fileSchema{
		Version:  currentSchemaVersion,
		Records:  records,
		Modified: s.now(),
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("computeruse: encode approvals: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("computeruse: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("computeruse: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("computeruse: close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("computeruse: replace %s: %w", s.path, err)
	}
	return nil
}

// IsApproved is the synchronous check computer-use call sites must call before
// every action. Returns false for unknown, revoked, or zero AppRefs.
//
// This call is allocation-free in the common case so it is safe to call from
// hot paths.
func (s *Store) IsApproved(app AppRef) bool {
	if app.IsZero() {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[app.Key()]
	return ok && rec.Active()
}

// Lookup returns the stored record for app, if any. The boolean is false when
// no record (active or revoked) exists.
func (s *Store) Lookup(app AppRef) (ApprovalRecord, bool) {
	if app.IsZero() {
		return ApprovalRecord{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[app.Key()]
	return rec, ok
}

// Approve grants scope to app and persists the record. If the app already has
// an active or revoked record, the row is replaced with a fresh approval
// (clearing any previous revocation timestamp).
func (s *Store) Approve(app AppRef, scope Scope, note string) (ApprovalRecord, error) {
	if app.IsZero() {
		return ApprovalRecord{}, ErrEmptyApp
	}
	if scope == "" {
		scope = ScopeFull
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rec := ApprovalRecord{
		App:        app,
		ApprovedAt: s.now(),
		Scope:      scope,
		Note:       note,
	}
	s.records[app.Key()] = rec
	if err := s.save(); err != nil {
		// Roll back the in-memory mutation so the store stays consistent
		// with disk. This matters: a caller observing the error could still
		// follow up with IsApproved and we don't want a phantom approval.
		delete(s.records, app.Key())
		return ApprovalRecord{}, err
	}
	return rec, nil
}

// Revoke marks the existing approval as revoked. The row is kept on disk so
// users have an audit trail. Returns ErrUnknownApp if no record exists.
func (s *Store) Revoke(app AppRef) (ApprovalRecord, error) {
	if app.IsZero() {
		return ApprovalRecord{}, ErrEmptyApp
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.records[app.Key()]
	if !ok {
		return ApprovalRecord{}, ErrUnknownApp
	}
	if !rec.Active() {
		return rec, nil
	}
	now := s.now()
	rec.RevokedAt = &now
	prev := s.records[app.Key()]
	s.records[app.Key()] = rec
	if err := s.save(); err != nil {
		s.records[app.Key()] = prev
		return ApprovalRecord{}, err
	}
	return rec, nil
}

// List returns every record (active and revoked), sorted by app key. The
// returned slice is a copy and may be mutated by the caller.
func (s *Store) List() []ApprovalRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ApprovalRecord, 0, len(s.records))
	for _, rec := range s.records {
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].App.Key() < out[j].App.Key()
	})
	return out
}

// ActiveApprovals returns only the records that currently grant access.
func (s *Store) ActiveApprovals() []ApprovalRecord {
	all := s.List()
	out := all[:0]
	for _, rec := range all {
		if rec.Active() {
			out = append(out, rec)
		}
	}
	return out
}

// Confirm is a callback that prompts the user to approve an app. It returns
// the granted scope, an optional note for the audit trail, and an error if
// the user declines or cancels. CLI/TUI integrations supply their own
// implementation; tests can pass a deterministic stub.
type Confirm func(app AppRef) (scope Scope, note string, err error)

// RequestApproval is the high-level helper a computer-use entry point can
// call when an app is not yet approved. If the app already has an active
// approval, the existing record is returned without prompting. Otherwise
// confirm is invoked and (on success) the result is persisted.
//
// Pass a nil confirm to refuse approval automatically — useful in headless
// runs where prompting is not possible.
func (s *Store) RequestApproval(app AppRef, confirm Confirm) (ApprovalRecord, error) {
	if app.IsZero() {
		return ApprovalRecord{}, ErrEmptyApp
	}
	if rec, ok := s.Lookup(app); ok && rec.Active() {
		return rec, nil
	}
	if confirm == nil {
		return ApprovalRecord{}, ErrNotApproved
	}
	scope, note, err := confirm(app)
	if err != nil {
		return ApprovalRecord{}, err
	}
	return s.Approve(app, scope, note)
}

// EnsureApproved is the integration hook computer-use call sites must invoke
// before every action against app. It performs a fast IsApproved check and,
// if no approval exists, returns ErrNotApproved without prompting.
//
// TODO(#37,#40): wire from MCP request handler and capability adapters. The
// MCP runtime (#37) should call EnsureApproved as the first step of
// dispatching any computer-use tool call; capability adapters (#40) must
// likewise gate every action behind this check. The function is intentionally
// allocation-free and concurrency-safe for use on hot paths.
func (s *Store) EnsureApproved(app AppRef) error {
	if s.IsApproved(app) {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrNotApproved, displayName(app))
}

func displayName(app AppRef) string {
	if app.Name != "" {
		if app.BundleID != "" {
			return fmt.Sprintf("%s (%s)", app.Name, app.BundleID)
		}
		return app.Name
	}
	if app.BundleID != "" {
		return app.BundleID
	}
	return "<empty app>"
}
