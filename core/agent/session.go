package agent

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/justin06lee/caveira/core/config"
	"github.com/justin06lee/caveira/core/llm"
)

// Session is a conversation on disk, so a run can be picked up later.
type Session struct {
	ID      string `json:"id"`
	WorkDir string `json:"work_dir"`
	Model   string `json:"model"`
	Title   string `json:"title"`
	// Source names the app a chat was imported from ("Claude Code"),
	// empty for one caveira started.
	Source    string        `json:"source,omitempty"`
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
	_ = s.Save()
}

// Save writes the session to its file.
func (s *Session) Save() error {
	if err := os.MkdirAll(sessionsDir(), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		return err
	}
	tmp := s.Path() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path())
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

// HasSession says whether a session with this id is saved.
func HasSession(id string) bool {
	if id == "" || strings.ContainsAny(id, `/\`) {
		return false
	}
	_, err := os.Stat(filepath.Join(sessionsDir(), id+".json"))
	return err == nil
}

// DeleteSession removes one session's file.
func DeleteSession(id string) error {
	if id == "" || strings.ContainsAny(id, `/\`) {
		return errors.New("not a session id: " + id)
	}
	return os.Remove(filepath.Join(sessionsDir(), id+".json"))
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
// newest first. Messages are not loaded, or read: an imported chat can be
// megabytes, and the sidebar asks for this list after every turn.
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
		s, err := readHeader(filepath.Join(sessionsDir(), e.Name()))
		if err != nil || s.ID == "" {
			continue
		}
		if workDir != "" && s.WorkDir != workDir {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// readHeader reads a session file as far as its messages. Save writes the
// header fields first, so that is all of them but the totals.
func readHeader(path string) (Session, error) {
	var s Session
	f, err := os.Open(path)
	if err != nil {
		return s, err
	}
	defer f.Close()
	dec := json.NewDecoder(bufio.NewReader(f))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return s, errors.New("not a session")
	}
	fields := map[string]any{
		"id":         &s.ID,
		"work_dir":   &s.WorkDir,
		"model":      &s.Model,
		"title":      &s.Title,
		"source":     &s.Source,
		"created_at": &s.CreatedAt,
		"updated_at": &s.UpdatedAt,
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return s, err
		}
		key, _ := t.(string)
		if key == "messages" && s.ID != "" {
			return s, nil
		}
		dst, ok := fields[key]
		if !ok {
			dst = new(json.RawMessage)
		}
		if err := dec.Decode(dst); err != nil {
			return s, err
		}
	}
	return s, nil
}
