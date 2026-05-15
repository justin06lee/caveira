package fabricate

import (
	"testing"

	"github.com/justin06lee/caveira/internal/walk"
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

func TestDiscoverIdentities(t *testing.T) {
	repo, _ := walk.MakeFixtureLinear(t, 3, []int{1, 1, 1})
	got, err := DiscoverIdentities(repo)
	if err != nil {
		t.Fatalf("DiscoverIdentities: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 unique identity, got %d: %+v", len(got), got)
	}
	if got[0].Name != "Test" || got[0].Email != "test@example.com" {
		t.Errorf("unexpected identity: %+v", got[0])
	}
}
