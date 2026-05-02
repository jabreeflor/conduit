// Package sessions persists Conduit conversation sessions as JSONL files
// with parent-child links between turns, enabling Git-for-conversations
// semantics: fork from any turn, replay from any checkpoint, and browse
// the resulting tree in any surface.
//
// Storage layout:
//
//	~/.conduit/sessions/
//	  <session-id>.jsonl   # one Turn per line; first line is the root meta turn.
//
// Every Turn carries a stable ID and an optional ParentID. A nil ParentID
// indicates the root of a session. Forks create a new session whose root
// turn copies the source turn's ID into its ParentID; this preserves the
// fork link across files so the tree can be reconstructed by walking the
// directory and following ParentID across sessions.
package sessions

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
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

const (
	// defaultSubdir matches PRD §6.13 layout (~/.conduit/sessions/).
	defaultSubdir = "sessions"
	dirPerm       = 0o700
	filePerm      = 0o600
)

// Turn is one entry persisted to a session JSONL file. The schema is
// intentionally close to PRD §6.13: {id, parentId, role, content,
// timestamp, metadata}, with extra fields (Model, Params) so replays can
// reconstruct the exact inference parameters.
type Turn struct {
	ID        string            `json:"id"`
	ParentID  string            `json:"parent_id,omitempty"`
	SessionID string            `json:"session_id"`
	Role      string            `json:"role"`
	Content   string            `json:"content"`
	Model     string            `json:"model,omitempty"`
	Params    map[string]string `json:"params,omitempty"`
	At        time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// Session is one JSONL file: a logical conversation thread. The root turn
// always has empty ParentID (when the session is itself a root) or carries
// the source turn's ID (when this session was created via Fork).
type Session struct {
	ID        string
	Path      string
	StartedAt time.Time
	// ForkParentID is set when this session was forked from another
	// session's turn; empty for natural root sessions.
	ForkParentID string
	Turns        []Turn

	mu sync.Mutex
}

// Store is the on-disk repository of all sessions. A single Store instance
// can be safely reused; per-session writes are serialized via the Session
// mutex.
type Store struct {
	dir string
}

// NewStore opens (or creates) the sessions directory under the given root.
// Pass an empty rootDir to use ~/.conduit/sessions.
func NewStore(rootDir string) (*Store, error) {
	if rootDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("sessions: resolve home: %w", err)
		}
		rootDir = filepath.Join(home, ".conduit", defaultSubdir)
	}
	if err := os.MkdirAll(rootDir, dirPerm); err != nil {
		return nil, fmt.Errorf("sessions: create dir: %w", err)
	}
	return &Store{dir: rootDir}, nil
}

// Dir returns the on-disk directory backing the store.
func (s *Store) Dir() string { return s.dir }

// Create starts a brand-new session. The session has no turns and no
// fork parent.
func (s *Store) Create() (*Session, error) {
	id, err := newSessionID()
	if err != nil {
		return nil, err
	}
	return &Session{
		ID:        id,
		Path:      filepath.Join(s.dir, id+".jsonl"),
		StartedAt: time.Now().UTC(),
	}, nil
}

// Append persists turn to disk and returns the assigned turn id. If
// turn.ID is empty a new id is generated. If turn.At is zero, time.Now is
// used. The on-disk file is created lazily so an aborted session never
// leaves a zero-byte journal.
func (s *Store) Append(sess *Session, turn Turn) (Turn, error) {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	if turn.At.IsZero() {
		turn.At = time.Now().UTC()
	}
	if turn.ID == "" {
		id, err := newTurnID(turn.At)
		if err != nil {
			return Turn{}, err
		}
		turn.ID = id
	}
	turn.SessionID = sess.ID

	line, err := json.Marshal(turn)
	if err != nil {
		return Turn{}, fmt.Errorf("sessions: marshal turn: %w", err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(sess.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, filePerm)
	if err != nil {
		return Turn{}, fmt.Errorf("sessions: open journal: %w", err)
	}
	if _, err := f.Write(line); err != nil {
		f.Close()
		return Turn{}, fmt.Errorf("sessions: write turn: %w", err)
	}
	if err := f.Close(); err != nil {
		return Turn{}, fmt.Errorf("sessions: close journal: %w", err)
	}

	sess.Turns = append(sess.Turns, turn)
	return turn, nil
}

// Load reads a previously written session JSONL into memory. Returns
// os.ErrNotExist (wrapped) when the session file does not exist.
func (s *Store) Load(id string) (*Session, error) {
	if id == "" {
		return nil, errors.New("sessions: load requires non-empty id")
	}
	path := filepath.Join(s.dir, id+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("sessions: open %s: %w", id, err)
	}
	defer f.Close()

	sess := &Session{ID: id, Path: path}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var t Turn
		if err := json.Unmarshal([]byte(line), &t); err != nil {
			return nil, fmt.Errorf("sessions: decode turn: %w", err)
		}
		if sess.StartedAt.IsZero() || t.At.Before(sess.StartedAt) {
			sess.StartedAt = t.At
		}
		// The first turn is treated as the session root for fork tracking;
		// its ParentID, when set, points at the source turn we forked from.
		if len(sess.Turns) == 0 && t.ParentID != "" {
			sess.ForkParentID = t.ParentID
		}
		sess.Turns = append(sess.Turns, t)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("sessions: scan journal: %w", err)
	}
	return sess, nil
}

// SessionInfo is the lightweight summary returned by List for slash
// commands and the TUI tree browser. It does not load every turn into
// memory; it just stats the file and reads the first/last turn lazily.
type SessionInfo struct {
	ID           string
	Path         string
	StartedAt    time.Time
	UpdatedAt    time.Time
	TurnCount    int
	ForkParentID string
	Title        string // first user content snippet, for human-readable lists
}

// List returns one SessionInfo per session file in the store directory,
// sorted by UpdatedAt descending (most recent first).
func (s *Store) List() ([]SessionInfo, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("sessions: read dir: %w", err)
	}
	var infos []SessionInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".jsonl")
		sess, err := s.Load(id)
		if err != nil {
			// Skip unreadable files rather than failing the whole list;
			// callers can still load known-good sessions individually.
			continue
		}
		info := SessionInfo{
			ID:           sess.ID,
			Path:         sess.Path,
			StartedAt:    sess.StartedAt,
			TurnCount:    len(sess.Turns),
			ForkParentID: sess.ForkParentID,
		}
		if len(sess.Turns) > 0 {
			info.UpdatedAt = sess.Turns[len(sess.Turns)-1].At
			for _, t := range sess.Turns {
				if t.Role == "user" && t.Content != "" {
					info.Title = truncate(t.Content, 60)
					break
				}
			}
		}
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].UpdatedAt.After(infos[j].UpdatedAt)
	})
	return infos, nil
}

// FindTurn locates a turn by ID across all sessions in the store. The
// returned session is the file the turn lives in. Returns os.ErrNotExist
// (wrapped) when no session contains the turn.
func (s *Store) FindTurn(turnID string) (*Session, Turn, error) {
	if turnID == "" {
		return nil, Turn{}, errors.New("sessions: turn id required")
	}
	infos, err := s.List()
	if err != nil {
		return nil, Turn{}, err
	}
	for _, info := range infos {
		sess, err := s.Load(info.ID)
		if err != nil {
			continue
		}
		for _, t := range sess.Turns {
			if t.ID == turnID {
				return sess, t, nil
			}
		}
	}
	return nil, Turn{}, fmt.Errorf("sessions: turn %q: %w", turnID, os.ErrNotExist)
}

// Fork creates a new session whose ancestry chain copies the source
// session's history up to and including parentTurnID. The new session's
// first turn carries ParentID=parentTurnID so the fork link survives a
// reload from disk. The new session is persisted before return.
func (s *Store) Fork(srcID, parentTurnID string) (*Session, error) {
	src, err := s.Load(srcID)
	if err != nil {
		return nil, err
	}
	prefix, ok := historyUpTo(src.Turns, parentTurnID)
	if !ok {
		return nil, fmt.Errorf("sessions: turn %q not found in session %q", parentTurnID, srcID)
	}

	dst, err := s.Create()
	if err != nil {
		return nil, err
	}
	dst.ForkParentID = parentTurnID

	// Replay the prefix into the new session, rewriting parent pointers so
	// the new session is internally consistent. The new root carries the
	// original parentTurnID as its ParentID — the only cross-session link.
	idMap := map[string]string{}
	for i, t := range prefix {
		newTurn := Turn{
			Role:     t.Role,
			Content:  t.Content,
			Model:    t.Model,
			Params:   cloneStringMap(t.Params),
			Metadata: cloneStringMap(t.Metadata),
			At:       time.Now().UTC(),
		}
		if i == 0 {
			newTurn.ParentID = parentTurnID
		} else {
			newTurn.ParentID = idMap[t.ParentID]
		}
		written, err := s.Append(dst, newTurn)
		if err != nil {
			return nil, err
		}
		idMap[t.ID] = written.ID
	}
	return dst, nil
}

// ReplayResponder is the model adapter used during Replay; it returns a
// new assistant turn for the given history. Tests inject a deterministic
// stub; real surfaces inject a provider client.
type ReplayResponder interface {
	Respond(history []Turn, model string, params map[string]string) (string, error)
}

// ReplayOptions configures a Replay run.
type ReplayOptions struct {
	Model     string
	Params    map[string]string
	Responder ReplayResponder
}

// Replay forks from parentTurnID and re-issues the next assistant turn
// using the provided responder. The forked session is persisted with the
// new assistant turn appended; both are returned so callers can navigate
// straight into the new branch.
func (s *Store) Replay(srcID, parentTurnID string, opts ReplayOptions) (*Session, Turn, error) {
	if opts.Responder == nil {
		return nil, Turn{}, errors.New("sessions: replay requires a Responder")
	}
	dst, err := s.Fork(srcID, parentTurnID)
	if err != nil {
		return nil, Turn{}, err
	}
	output, err := opts.Responder.Respond(dst.Turns, opts.Model, opts.Params)
	if err != nil {
		return nil, Turn{}, fmt.Errorf("sessions: replay respond: %w", err)
	}
	parent := ""
	if len(dst.Turns) > 0 {
		parent = dst.Turns[len(dst.Turns)-1].ID
	}
	turn, err := s.Append(dst, Turn{
		Role:     "assistant",
		Content:  output,
		Model:    opts.Model,
		Params:   cloneStringMap(opts.Params),
		ParentID: parent,
		Metadata: map[string]string{"source": "replay"},
	})
	if err != nil {
		return nil, Turn{}, err
	}
	return dst, turn, nil
}

// historyUpTo returns the prefix of turns up to and including the turn
// with id matchID, walking ancestry instead of slice order so out-of-order
// branches (created via fork) still produce a valid linear history.
func historyUpTo(turns []Turn, matchID string) ([]Turn, bool) {
	idx := map[string]Turn{}
	for _, t := range turns {
		idx[t.ID] = t
	}
	target, ok := idx[matchID]
	if !ok {
		return nil, false
	}
	// Walk up the parent chain to the root, then reverse.
	var chain []Turn
	cur := target
	for {
		chain = append(chain, cur)
		if cur.ParentID == "" {
			break
		}
		next, ok := idx[cur.ParentID]
		if !ok {
			break
		}
		cur = next
	}
	// reverse
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, true
}

func cloneStringMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func newSessionID() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("sessions: random session id: %w", err)
	}
	return fmt.Sprintf("sess-%d-%s", time.Now().UTC().Unix(), hex.EncodeToString(buf)), nil
}

func newTurnID(at time.Time) (string, error) {
	buf := make([]byte, 3)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("sessions: random turn id: %w", err)
	}
	return fmt.Sprintf("turn-%d-%s", at.UTC().UnixNano(), hex.EncodeToString(buf)), nil
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}
