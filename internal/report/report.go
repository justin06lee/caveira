package report

import (
	"fmt"
	"io"
	"strings"
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
	fmt.Fprintf(w, "\nSpan: %s (window: %s). Scale: s=%s. Squashes: %d.\n",
		roundDur(span), roundDur(window), formatScale(res.Scale), len(res.Squashes))
}

// roundDur rounds a duration for display. Narrow windows (typical of
// --preserve, where the whole history is compressed into minutes or seconds)
// are rounded to the second so sub-minute spacing stays visible; wider spans
// round to the minute to keep long retimes readable.
func roundDur(d time.Duration) time.Duration {
	if d < 10*time.Minute {
		return d.Round(time.Second)
	}
	return d.Round(time.Minute)
}

// formatScale prints the compression scale. Two decimals suffice for normal
// retimes, but --preserve can drive the scale well below 0.01 (compressing a
// long history into a tiny window), where "%.2f" would collapse to "0.00".
// Such tiny scales fall back to three significant figures so the value stays
// informative. scale==1.0 still renders as "1.00".
func formatScale(s float64) string {
	if s == 0 || s >= 0.01 {
		return fmt.Sprintf("%.2f", s)
	}
	return fmt.Sprintf("%.3g", s)
}

// WriteSummary prints the post-run summary. tags is the number of tags written
// (0 hides the line — e.g. retime, which preserves tags without counting them).
func WriteSummary(w io.Writer, src, dst, deadPath string, before, after int, span, window time.Duration, scale float64, squashes, tags int, pushed bool) {
	fmt.Fprintf(w, "Source:        %s\n", src)
	fmt.Fprintf(w, "Rewritten:     %s\n", dst)
	fmt.Fprintf(w, "Original kept: %s\n", deadPath)
	fmt.Fprintf(w, "Commits:       %d -> %d (%d squashed)\n", before, after, squashes)
	if tags > 0 {
		fmt.Fprintf(w, "Tags:          %d\n", tags)
	}
	fmt.Fprintf(w, "Span:          %s within %s window\n", roundDur(span), roundDur(window))
	fmt.Fprintf(w, "Scaling:       s=%s\n", formatScale(scale))
	if pushed {
		fmt.Fprintln(w, "Pushed:        yes")
	} else {
		fmt.Fprintln(w, "Pushed:        no")
	}
}

// WriteTagList prints the fabricated release tags for the dry-run preview.
// Nothing is printed when names is empty.
func WriteTagList(w io.Writer, names []string) {
	if len(names) == 0 {
		return
	}
	fmt.Fprintf(w, "Tags: %d (%s)\n", len(names), strings.Join(names, ", "))
}
