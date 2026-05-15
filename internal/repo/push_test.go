package repo

import "testing"

func TestIsProtectedBranch(t *testing.T) {
	cases := map[string]bool{
		"main":            true,
		"master":          true,
		"refs/heads/main": true,
		"feature/x":       false,
		"refs/heads/dev":  false,
	}
	for in, want := range cases {
		if got := IsProtectedBranch(in); got != want {
			t.Errorf("IsProtectedBranch(%q) = %v, want %v", in, got, want)
		}
	}
}
