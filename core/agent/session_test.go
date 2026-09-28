package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/justin06lee/caveira/core/llm"
)

func TestListSessionsReadsHeadersOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := time.Now()
	older := &Session{ID: "a", WorkDir: "/p", Title: "first", CreatedAt: now, UpdatedAt: now.Add(-time.Hour),
		Messages: []llm.Message{{Role: llm.RoleUser, Content: strings.Repeat("x", 1<<20)}}}
	newer := &Session{ID: "b", WorkDir: "/p", Title: "second", Source: "Claude Code", CreatedAt: now, UpdatedAt: now,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, Totals: Totals{CostUSD: 1}}
	other := &Session{ID: "c", WorkDir: "/q", Title: "elsewhere", UpdatedAt: now,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}}}
	for _, s := range []*Session{older, newer, other} {
		if err := s.Save(); err != nil {
			t.Fatal(err)
		}
	}

	got, err := ListSessions("/p")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "b" || got[1].ID != "a" {
		t.Fatalf("want b then a, got %+v", got)
	}
	if got[0].Title != "second" || got[0].Source != "Claude Code" || got[0].WorkDir != "/p" {
		t.Errorf("header not read: %+v", got[0])
	}
	if got[1].Messages != nil {
		t.Error("messages were loaded")
	}

	full, err := LoadSession("b")
	if err != nil {
		t.Fatal(err)
	}
	if full.Totals.CostUSD != 1 || len(full.Messages) != 1 || full.Source != "Claude Code" {
		t.Errorf("round trip lost something: %+v", full)
	}

	if !HasSession("b") || HasSession("nope") || HasSession("../b") {
		t.Error("HasSession is wrong")
	}

	all, _ := ListSessions("")
	if len(all) != 3 {
		t.Errorf("want 3 sessions in all, got %d", len(all))
	}
}
