package report

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/justin06lee/caveira/internal/difficulty"
	"github.com/justin06lee/caveira/internal/schedule"
)

// Row describes one commit in a dry-run table.
type Row struct {
	ShortOID   string
	Difficulty difficulty.Difficulty
	Duration   int
	OldTime    time.Time
	NewTime    time.Time
}

// WriteDryRun prints a table of commits and a summary footer.
func WriteDryRun(w io.Writer, rows []Row, res *schedule.Result, windowStart, windowEnd time.Time) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SHA\tDifficulty\tDuration\tOriginal\tNew")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%dm\t%s\t%s\n",
			r.ShortOID, r.Difficulty, r.Duration,
			r.OldTime.Format(time.RFC3339), r.NewTime.Format(time.RFC3339))
	}
	tw.Flush()
	span := time.Duration(0)
	for _, t := range res.NewTimes {
		if d := t.Sub(windowStart); d > span {
			span = d
		}
	}
	window := windowEnd.Sub(windowStart)
	fmt.Fprintf(w, "\nSpan: %s (window: %s). Scale: s=%.2f. Squashes: %d.\n",
		span.Round(time.Minute), window.Round(time.Minute), res.Scale, len(res.Squashes))
}

// WriteSummary prints the post-run summary.
func WriteSummary(w io.Writer, src, dst, deadPath string, before, after int, span, window time.Duration, scale float64, squashes int, pushed bool) {
	fmt.Fprintf(w, "Source:        %s\n", src)
	fmt.Fprintf(w, "Rewritten:     %s\n", dst)
	fmt.Fprintf(w, "Original kept: %s\n", deadPath)
	fmt.Fprintf(w, "Commits:       %d -> %d (%d squashed)\n", before, after, squashes)
	fmt.Fprintf(w, "Span:          %s within %s window\n", span.Round(time.Minute), window.Round(time.Minute))
	fmt.Fprintf(w, "Scaling:       s=%.2f\n", scale)
	if pushed {
		fmt.Fprintln(w, "Pushed:        yes")
	} else {
		fmt.Fprintln(w, "Pushed:        no")
	}
}
