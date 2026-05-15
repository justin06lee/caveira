package fabricate

import (
	"testing"
)

func TestParseIdentity_Valid(t *testing.T) {
	cases := []struct {
		in   string
		want Identity
	}{
		{"Alice <a@x.com>", Identity{Name: "Alice", Email: "a@x.com"}},
		{"Alice Cooper <alice.cooper@example.com>", Identity{Name: "Alice Cooper", Email: "alice.cooper@example.com"}},
		{"  Bob   <bob@y.com>  ", Identity{Name: "Bob", Email: "bob@y.com"}},
	}
	for _, c := range cases {
		got, err := ParseIdentity(c.in)
		if err != nil {
			t.Errorf("ParseIdentity(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseIdentity(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestParseIdentity_Invalid(t *testing.T) {
	cases := []string{
		"",
		"Alice",
		"a@x.com",
		"<a@x.com>",
		"Alice <>",
		"Alice <noatsign>",
	}
	for _, in := range cases {
		if _, err := ParseIdentity(in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}
