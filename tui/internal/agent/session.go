package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/justin06lee/caveira/tui/internal/config"
	"github.com/justin06lee/caveira/tui/internal/llm"
)

// Session is a conversation on disk, so a run can be picked up later.
type Session struct {
	ID        string        `json:"id"`
	WorkDir   string        `json:"work_dir"`
	Model     string        `json:"model"`
	Title     string        `json:"title"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Messages  []llm.Message `json:"messages"`
	Totals    Totals        `json:"totals"`
}

func sessionsDir() string { return filepath.Join(config.Dir(), "sessions") }

// Path is the session's file.
func (s *Session) Path() string { return filepath.Join(sessionsDir(), s.ID+".json") }

// NewSession starts an empty session for the agent. Nothing is written until
// the first turn completes.
func (a *Agent) NewSession() *Session {
	var rnd [3]byte
	_, _ = rand.Read(rnd[:])
	now := time.Now()
	a.Session = &Session{
		ID:        now.Format("20060102-150405") + "-" + hex.EncodeToString(rnd[:]),
		WorkDir:   a.WorkDir,
		Model:     a.Model,
		CreatedAt: now,
	}
	return a.Session
}

// Attach continues a stored session.
func (a *Agent) Attach(s *Session) {
	a.Session = s
	a.Messages = s.Messages
	a.Totals = s.Totals
	if s.Model != "" && s.Model != a.Model {
		// The model the session was recorded with is kept in the file for
		// reference; the current configuration decides what runs now.
		s.Model = a.Model
	}
}

func (a *Agent) save() {
	if a.Session == nil {
		return
	}
	s := a.Session
	s.Messages = a.Messages
	s.Totals = a.Totals
	s.Model = a.Model
	s.UpdatedAt = time.Now()
	if s.Title == "" {
		for _, m := range a.Messages {
			if m.Role == llm.RoleUser {
				s.Title = firstLine(m.Content, 80)
				break
			}
		}
	}
	if len(s.Messages) == 0 {
		return
	}
	if err := os.MkdirAll(sessionsDir(), 0o700); err != nil {
		return
	}
	b, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		return
	}
	tmp := s.Path() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, s.Path())
}

func firstLine(s string, n int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > n {
		s = s[:n] + "…"
	}
	return s
}

// LoadSession reads one session by ID.
func LoadSession(id string) (*Session, error) {
	b, err := os.ReadFile(filepath.Join(sessionsDir(), id+".json"))
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// LatestSession finds the most recently updated session for workDir.
func LatestSession(workDir string) (*Session, error) {
	sessions, err := ListSessions(workDir)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, errors.New("no previous session for this directory")
	}
	return LoadSession(sessions[0].ID)
}

// ListSessions returns session headers for workDir (or all when empty),
// newest first. Messages are not loaded.
func ListSessions(workDir string) ([]Session, error) {
	entries, err := os.ReadDir(sessionsDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Session
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(sessionsDir(), e.Name()))
		if err != nil {
			continue
		}
		var s Session
		if json.Unmarshal(b, &s) != nil {
			continue
		}
		if workDir != "" && s.WorkDir != workDir {
			continue
		}
		s.Messages = nil
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
