package repo

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
)

// IsURL returns true if s looks like a clonable URL.
func IsURL(s string) bool {
	if strings.HasPrefix(s, "git@") {
		return true
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "git" || u.Scheme == "ssh"
}

// CloneURL clones repoURL into parentDir/<basename>, returning the path.
func CloneURL(repoURL, parentDir string) (string, error) {
	if parentDir == "" {
		var err error
		parentDir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	name := basenameFromURL(repoURL)
	dst := filepath.Join(parentDir, name)
	if _, err := os.Stat(dst); err == nil {
		return "", fmt.Errorf("destination %s already exists", dst)
	}
	if _, err := git.PlainClone(dst, false, &git.CloneOptions{URL: repoURL}); err != nil {
		// PlainClone may leave a partial directory behind on failure. dst was
		// confirmed not to pre-exist above, so removing it only discards what
		// this call created.
		os.RemoveAll(dst)
		return "", fmt.Errorf("clone %s: %w", repoURL, err)
	}
	return dst, nil
}

func basenameFromURL(s string) string {
	s = strings.TrimSuffix(s, "/")
	idx := strings.LastIndex(s, "/")
	if idx < 0 {
		return strings.TrimSuffix(s, ".git")
	}
	last := s[idx+1:]
	return strings.TrimSuffix(last, ".git")
}
