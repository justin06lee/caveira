package fabricate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMailmap_NilSafe(t *testing.T) {
	var mm *Mailmap
	id := Identity{Name: "Bob", Email: "bob@example.com"}
	if got := mm.Canonical(id); got != id {
		t.Fatalf("nil Mailmap should pass through, got %+v", got)
	}
}

func TestMailmap_Form1_NameForEmail(t *testing.T) {
	mm := ParseMailmap([]byte("Proper Name <e@x.com>\n"))
	got := mm.Canonical(Identity{Name: "old name", Email: "e@x.com"})
	if got.Name != "Proper Name" || got.Email != "e@x.com" {
		t.Fatalf("form 1: got %+v", got)
	}
}

func TestMailmap_Form2_EmailRemap(t *testing.T) {
	mm := ParseMailmap([]byte("<proper@x.com> <commit@x.com>\n"))
	got := mm.Canonical(Identity{Name: "Bob", Email: "commit@x.com"})
	// form 2 keeps the commit name, remaps only the email.
	if got.Name != "Bob" || got.Email != "proper@x.com" {
		t.Fatalf("form 2: got %+v", got)
	}
}

func TestMailmap_Form3_NameAndEmailRemap(t *testing.T) {
	mm := ParseMailmap([]byte("Proper Name <proper@x.com> <commit@x.com>\n"))
	got := mm.Canonical(Identity{Name: "whatever", Email: "commit@x.com"})
	if got.Name != "Proper Name" || got.Email != "proper@x.com" {
		t.Fatalf("form 3: got %+v", got)
	}
	// a commit already under the proper email also gets the proper name.
	got2 := mm.Canonical(Identity{Name: "x", Email: "proper@x.com"})
	if got2.Name != "Proper Name" {
		t.Fatalf("form 3 proper-email name: got %+v", got2)
	}
}

func TestMailmap_Form4_NameSpecific(t *testing.T) {
	mm := ParseMailmap([]byte("Proper Name <proper@x.com> Commit Name <commit@x.com>\n"))
	hit := mm.Canonical(Identity{Name: "Commit Name", Email: "commit@x.com"})
	if hit.Name != "Proper Name" || hit.Email != "proper@x.com" {
		t.Fatalf("form 4 match: got %+v", hit)
	}
	// a different name on the same commit email must NOT be remapped.
	miss := mm.Canonical(Identity{Name: "Someone Else", Email: "commit@x.com"})
	if miss.Name != "Someone Else" || miss.Email != "commit@x.com" {
		t.Fatalf("form 4 non-match should pass through: got %+v", miss)
	}
}

func TestMailmap_CommentsAndCaseInsensitive(t *testing.T) {
	mm := ParseMailmap([]byte("# a comment\n\nProper Name <proper@x.com> <Commit@X.com>\n"))
	got := mm.Canonical(Identity{Name: "x", Email: "commit@x.COM"})
	if got.Email != "proper@x.com" {
		t.Fatalf("case-insensitive email match failed: got %+v", got)
	}
}

func TestMailmap_Unmapped(t *testing.T) {
	mm := ParseMailmap([]byte("Proper Name <proper@x.com> <commit@x.com>\n"))
	id := Identity{Name: "Stranger", Email: "stranger@elsewhere.com"}
	if got := mm.Canonical(id); got != id {
		t.Fatalf("unmapped identity should pass through, got %+v", got)
	}
}

func TestLoadMailmap(t *testing.T) {
	dir := t.TempDir()
	// Absent file -> nil, no error.
	mm, err := LoadMailmap(dir)
	if err != nil {
		t.Fatalf("LoadMailmap absent: %v", err)
	}
	if mm.Canonical(Identity{Name: "a", Email: "b@c"}) != (Identity{Name: "a", Email: "b@c"}) {
		t.Fatal("absent mailmap should be a passthrough")
	}
	// Present file -> parsed.
	if err := os.WriteFile(filepath.Join(dir, ".mailmap"),
		[]byte("Proper <proper@x.com> <commit@x.com>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mm, err = LoadMailmap(dir)
	if err != nil {
		t.Fatalf("LoadMailmap present: %v", err)
	}
	if got := mm.Canonical(Identity{Name: "x", Email: "commit@x.com"}); got.Email != "proper@x.com" {
		t.Fatalf("loaded mailmap not applied: got %+v", got)
	}
}
