package importer

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/justin06lee/caveira/core/agent"
)

// TestThisMachine reads every chat the agents on this machine left, without
// writing anything, and reports what came of it:
//
//	CAVEIRA_IMPORT_REAL=1 go test -run TestThisMachine -v ./importer
func TestThisMachine(t *testing.T) {
	if os.Getenv("CAVEIRA_IMPORT_REAL") == "" {
		t.Skip("set CAVEIRA_IMPORT_REAL=1 to read this machine's chats")
	}
	home, _ := os.UserHomeDir()
	start := time.Now()
	chats := Scan(context.Background(), home)
	t.Logf("scan: %d chats in %s", len(chats), time.Since(start).Round(time.Millisecond))
	type tally struct{ ok, empty, failed, msgs, bytes, nomodel int }
	by := map[string]*tally{}
	start = time.Now()
	Each(context.Background(), chats, func(c Chat, s *agent.Session, err error) {
		tl := by[c.App.Name]
		if tl == nil {
			tl = &tally{}
			by[c.App.Name] = tl
		}
		switch {
		case errors.Is(err, ErrEmpty):
			tl.empty++
		case err != nil:
			tl.failed++
			t.Logf("%s %s: %v", c.App.Name, c.ID, err)
		default:
			tl.ok++
			tl.msgs += len(s.Messages)
			if s.Model == "" {
				tl.nomodel++
			}
			for _, m := range s.Messages {
				tl.bytes += len(m.Content) + len(m.Reasoning)
			}
		}
	})
	for name, tl := range by {
		t.Logf("%s: %d chats, %d empty, %d failed, %d messages, %d KB, %d without a model", name, tl.ok, tl.empty, tl.failed, tl.msgs, tl.bytes>>10, tl.nomodel)
	}
	t.Logf("convert: %s", time.Since(start).Round(time.Millisecond))
}
