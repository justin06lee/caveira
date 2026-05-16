package fabricate

import "testing"

func TestParseCoAuthors(t *testing.T) {
	msg := "feat: add thing\n\n" +
		"Some body text.\n\n" +
		"Co-Authored-By: Claude <noreply@anthropic.com>\n" +
		"co-authored-by: Bob Jones <bob@example.com>\n"
	got := parseCoAuthors(msg)
	if len(got) != 2 {
		t.Fatalf("got %d co-authors, want 2: %+v", len(got), got)
	}
	if got[0].Name != "Claude" || got[0].Email != "noreply@anthropic.com" {
		t.Errorf("co-author 0 = %+v", got[0])
	}
	if got[1].Name != "Bob Jones" || got[1].Email != "bob@example.com" {
		t.Errorf("co-author 1 = %+v", got[1])
	}
}

func TestParseCoAuthors_None(t *testing.T) {
	if got := parseCoAuthors("feat: a plain commit\n\nno trailers here"); len(got) != 0 {
		t.Fatalf("expected no co-authors, got %+v", got)
	}
}

func TestParseCoAuthors_SkipsInvalid(t *testing.T) {
	msg := "feat: add thing\n\n" +
		"Co-Authored-By: no brackets here\n" +
		"Co-Authored-By: Real Person <real@example.com>\n"
	got := parseCoAuthors(msg)
	if len(got) != 1 {
		t.Fatalf("got %d co-authors, want 1: %+v", len(got), got)
	}
	if got[0].Name != "Real Person" || got[0].Email != "real@example.com" {
		t.Errorf("co-author 0 = %+v", got[0])
	}
}
