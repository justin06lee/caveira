package repo

import (
	"fmt"
	"os"
)

// FindNextDead returns the first non-existing path among:
//   <orig>.dead, <orig>.dead.1, <orig>.dead.2, ...
func FindNextDead(orig string) string {
	candidate := orig + ".dead"
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s.dead.%d", orig, i)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// Swap renames orig -> dead (auto-versioned), then stage -> orig.
// On failure of the second rename, attempts to roll back the first.
// Returns the dead path actually used.
func Swap(orig, stage string) (string, error) {
	if _, err := os.Stat(stage); err != nil {
		return "", fmt.Errorf("staging path %s missing: %w", stage, err)
	}
	dead := FindNextDead(orig)
	if err := os.Rename(orig, dead); err != nil {
		return "", fmt.Errorf("rename orig -> dead: %w", err)
	}
	if err := os.Rename(stage, orig); err != nil {
		// Roll back.
		if rbErr := os.Rename(dead, orig); rbErr != nil {
			return "", fmt.Errorf("rename stage -> orig failed (%v); rollback also failed (%v); orig is now at %s",
				err, rbErr, dead)
		}
		return "", fmt.Errorf("rename stage -> orig: %w", err)
	}
	return dead, nil
}
