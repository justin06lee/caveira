package repo

import "testing"

func TestIsURL(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"https://github.com/u/r.git", true},
		{"git@github.com:u/r.git", true},
		{"/path/to/local", false},
		{"./relative", false},
	}
	for _, c := range cases {
		if got := IsURL(c.in); got != c.want {
			t.Errorf("IsURL(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestBasenameFromURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/u/myrepo.git": "myrepo",
		"https://github.com/u/myrepo":     "myrepo",
		"git@github.com:u/myrepo.git":     "myrepo",
	}
	for in, want := range cases {
		if got := basenameFromURL(in); got != want {
			t.Errorf("basenameFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}
