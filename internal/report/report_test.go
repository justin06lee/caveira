package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/justin06lee/caveira/internal/difficulty"
	"github.com/justin06lee/caveira/internal/schedule"
)

func TestWriteDryRun(t *testing.T) {
	var buf bytes.Buffer
	t0 := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	rows := []Row{
		{ShortOID: "abc1234", Difficulty: difficulty.Trivial, Duration: 4, OldTime: t0.Add(-time.Hour), NewTime: t0.Add(4 * time.Minute)},
		{ShortOID: "def5678", Difficulty: difficulty.Hard, Duration: 77, OldTime: t0, NewTime: t0.Add(81 * time.Minute)},
	}
	res := &schedule.Result{Scale: 0.85}
	WriteDryRun(&buf, rows, res, t0, t0.Add(4*time.Hour))
	out := buf.String()
	for _, want := range []string{"abc1234", "trivial", "Scale", "Span"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q; got: %s", want, out)
		}
	}
}
