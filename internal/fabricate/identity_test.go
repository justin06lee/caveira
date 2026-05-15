package fabricate

import (
	"bytes"
	"strings"
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

func TestResolveIdentities_AllFromFlags(t *testing.T) {
	repo, _ := walk.MakeFixtureLinear(t, 2, []int{1, 1})
	flags := []string{"Alice <a@x.com>", "Bob <b@x.com>"}
	got, err := ResolveIdentities(repo, flags, 2, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("ResolveIdentities: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 identities, got %d", len(got))
	}
	if got[0].Name != "Alice" || got[1].Name != "Bob" {
		t.Errorf("flag identities lost or reordered: %+v", got)
	}
}

func TestResolveIdentities_FillFromGit(t *testing.T) {
	repo, _ := walk.MakeFixtureLinear(t, 2, []int{1, 1})
	flags := []string{}
	got, err := ResolveIdentities(repo, flags, 1, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("ResolveIdentities: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 identity, got %d", len(got))
	}
	if got[0].Name != "Test" {
		t.Errorf("expected discovered identity, got %+v", got[0])
	}
}

func TestResolveIdentities_PromptWhenShort(t *testing.T) {
	// Fixture has 1 identity ("Test"). We need 3. Should prompt twice.
	repo, _ := walk.MakeFixtureLinear(t, 1, []int{1})
	flags := []string{}
	stdin := strings.NewReader("Bob\nbob@x.com\nCarol\ncarol@x.com\n")
	var stdout bytes.Buffer
	got, err := ResolveIdentities(repo, flags, 3, stdin, &stdout)
	if err != nil {
		t.Fatalf("ResolveIdentities: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 identities, got %d", len(got))
	}
	if got[1].Name != "Bob" || got[2].Name != "Carol" {
		t.Errorf("prompted identities incorrect: %+v", got)
	}
}

func TestResolveIdentities_PickerWhenTooMany(t *testing.T) {
	// This test verifies the picker code path when discovered > needed.
	repo, _ := walk.MakeFixtureLinear(t, 1, []int{1})
	flags := []string{}
	stdin := strings.NewReader("1\n")
	var stdout bytes.Buffer
	got, err := ResolveIdentities(repo, flags, 1, stdin, &stdout)
	if err != nil {
		t.Fatalf("ResolveIdentities: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 identity, got %d", len(got))
	}
}
