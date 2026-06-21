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

func TestFormatScale(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{1.0, "1.00"}, // integration tests depend on this exact rendering
		{0.85, "0.85"},
		{0.0, "0.00"},
		{0.006, "0.006"}, // tiny --preserve scale must not collapse to 0.00
		{0.00012, "0.00012"},
	}
	for _, c := range cases {
		if got := formatScale(c.in); got != c.want {
			t.Errorf("formatScale(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRoundDur(t *testing.T) {
	// Sub-minute spans (typical of --preserve) keep second resolution instead
	// of rounding away to "0s".
	if got := roundDur(45 * time.Second); got != 45*time.Second {
		t.Errorf("roundDur(45s) = %v, want 45s", got)
	}
	// Wide spans round to the minute for readability.
	if got := roundDur(2*time.Hour + 20*time.Second); got != 2*time.Hour {
		t.Errorf("roundDur(2h20s) = %v, want 2h0m0s", got)
	}
}
