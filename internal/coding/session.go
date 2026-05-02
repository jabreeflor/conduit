package coding

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

const (
	// sessionsSubdir keeps coding-agent journals separate from usage logs and
	// other ~/.conduit artefacts so a single session can be inspected without
	// cross-cutting noise.
	sessionsSubdir  = "coding-sessions"
	sessionDirPerm  = 0o700
	sessionFilePerm = 0o600
)

// Session is the append-only journal for one `conduit code` REPL run.
// Turns are kept in memory for in-process inspection (status surface,
// auto-continuation, in-flight compaction) and mirrored to a JSONL file
// for resumability across restarts.
type Session struct {
	ID        string
	StartedAt time.Time
	Turns     []contracts.CodingTurn
	Path      string

	mu sync.Mutex
	// homeDir is retained so future helpers (LoadSession variants, log
	// rotation) can re-derive paths without the caller threading it.
	homeDir string
}

// NewSession creates a session, ensures the journal directory exists, and
// reserves the JSONL path. The file itself is created lazily on first
// Append so an aborted REPL never leaves a zero-byte journal behind.
func NewSession(home string) (*Session, error) {
	if home == "" {
		return nil, fmt.Errorf("coding: home dir required")
	}
	id, err := newSessionID()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".conduit", sessionsSubdir)
	if err := os.MkdirAll(dir, sessionDirPerm); err != nil {
		return nil, fmt.Errorf("coding: create session dir: %w", err)
	}
	return &Session{
		ID:        id,
		StartedAt: time.Now().UTC(),
		Path:      filepath.Join(dir, id+".jsonl"),
		homeDir:   home,
	}, nil
}

// Append assigns a snapshot ID, persists the turn to the JSONL journal,
// and records it in the in-memory slice. Persistence happens before the
// in-memory append so a crash cannot leave the runtime believing in a
// turn that never made it to disk.
func (s *Session) Append(turn contracts.CodingTurn) (contracts.CodingSnapshotID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if turn.At.IsZero() {
		turn.At = time.Now().UTC()
	}
	if turn.Index == 0 {
		turn.Index = len(s.Turns)
	}
	id, err := newSnapshotID(turn.At)
	if err != nil {
		return "", err
	}
	turn.SnapshotID = id

	line, err := json.Marshal(turn)
	if err != nil {
		return "", fmt.Errorf("coding: marshal turn: %w", err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(s.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, sessionFilePerm)
	if err != nil {
		return "", fmt.Errorf("coding: open session journal: %w", err)
	}
	if _, err := f.Write(line); err != nil {
		f.Close()
		return "", fmt.Errorf("coding: write turn: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("coding: close session journal: %w", err)
	}

	s.Turns = append(s.Turns, turn)
	return id, nil
}

// LoadSession reads back a previously written journal so the REPL can
// resume mid-conversation. The journal is the source of truth — anything
// not on disk is gone.
func LoadSession(home, id string) (*Session, error) {
	if home == "" {
		return nil, fmt.Errorf("coding: home dir required")
	}
	if id == "" {
		return nil, fmt.Errorf("coding: session id required")
	}
	path := filepath.Join(home, ".conduit", sessionsSubdir, id+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("coding: open session journal: %w", err)
	}
	defer f.Close()

	session := &Session{
		ID:      id,
		Path:    path,
		homeDir: home,
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var turn contracts.CodingTurn
		if err := json.Unmarshal(scanner.Bytes(), &turn); err != nil {
			return nil, fmt.Errorf("coding: decode turn: %w", err)
		}
		if session.StartedAt.IsZero() || turn.At.Before(session.StartedAt) {
			session.StartedAt = turn.At
		}
		session.Turns = append(session.Turns, turn)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("coding: scan session journal: %w", err)
	}
	return session, nil
}

func newSessionID() (string, error) {
	buf := make([]byte, 3)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("coding: random session id: %w", err)
	}
	return fmt.Sprintf("code-%d-%s", time.Now().UTC().Unix(), hex.EncodeToString(buf)), nil
}

func newSnapshotID(t time.Time) (contracts.CodingSnapshotID, error) {
	buf := make([]byte, 3)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("coding: random snapshot id: %w", err)
	}
	return contracts.CodingSnapshotID(fmt.Sprintf("snap-%s-%s", t.UTC().Format(time.RFC3339), hex.EncodeToString(buf))), nil
}
