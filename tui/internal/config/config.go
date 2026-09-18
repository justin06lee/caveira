// Package config stores the CLI's credentials between runs.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Auth is what lives in ~/.caveira/auth.json.
type Auth struct {
	Token string `json:"token"`
	Email string `json:"email"`
}

// BaseURL is the caveira backend the CLI talks to. Overridable so a developer
// can point the binary at a local Next.js server without rebuilding it.
func BaseURL() string {
	if v := os.Getenv("CAVEIRA_API_URL"); v != "" {
		return v
	}
	return "https://caveira.dev"
}

func dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".caveira"), nil
}

// Path is where the token is kept, for messages that need to name it.
func Path() string {
	d, err := dir()
	if err != nil {
		return "~/.caveira/auth.json"
	}
	return filepath.Join(d, "auth.json")
}

// Load returns the saved credentials, or nil when there are none. A missing
// file is the normal first-run state, not an error.
func Load() (*Auth, error) {
	b, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var a Auth
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, err
	}
	if a.Token == "" {
		return nil, nil
	}
	return &a, nil
}

// Save writes the token 0600: it is a bearer credential with a year of life,
// and the home directory is not always private.
func Save(a Auth) error {
	d, err := dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, "auth.json"), append(b, '\n'), 0o600)
}

// Clear removes the saved credentials; used by sign out.
func Clear() error {
	err := os.Remove(Path())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
