package input

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/araddon/dateparse"
)

// ParseDateTime parses a user-supplied date/time string in the context of tz
// and a reference "now". Supports a handful of relative keywords ("now",
// "today", "tomorrow", "yesterday", optionally followed by a time of day) and
// otherwise defers to dateparse for absolute forms.
func ParseDateTime(s string, tz *time.Location, now time.Time) (time.Time, error) {
	trimmed := strings.TrimSpace(strings.ToLower(s))

	if t, ok := tryRelative(trimmed, tz, now); ok {
		return t, nil
	}

	return parseAbsolute(s, tz, now)
}

// ampmSuffixRe matches a trailing 12-hour time-of-day suffix like "1pm",
// "1:30 PM", "12am", etc. dateparse handles HH:MM (24-hour) fine but
// mishandles am/pm-suffixed times, so we strip and apply them ourselves.
var ampmSuffixRe = regexp.MustCompile(`(?i)\s+(\d{1,2})(?::(\d{2}))?\s*(am|pm)\s*$`)

func parseAbsolute(s string, tz *time.Location, now time.Time) (time.Time, error) {
	datePart := s
	var (
		hasTime bool
		hour    int
		minute  int
	)

	if loc := ampmSuffixRe.FindStringSubmatchIndex(s); loc != nil {
		sm := ampmSuffixRe.FindStringSubmatch(s)
		h, mn, ok := decodeAMPM(sm[1], sm[2], sm[3])
		if !ok {
			return time.Time{}, fmt.Errorf("could not parse time-of-day in %q", s)
		}
		hour, minute = h, mn
		hasTime = true
		datePart = strings.TrimSpace(s[:loc[0]])
	}

	// If datePart is empty here it means the input was just a time-of-day,
	// which we don't support; fall through to dateparse for a clear error.
	if datePart == "" {
		datePart = s
	}

	t, err := dateparse.ParseIn(datePart, tz)
	if err != nil {
		return time.Time{}, fmt.Errorf("could not parse date %q (try YYYY-MM-DD HH:MM): %w", s, err)
	}

	// dateparse returns year 0 for inputs like "5/14" or "May 14". Only then do
	// we substitute the year from "now"; trusting the parsed year otherwise
	// avoids silently clobbering a real year that isn't a word-bounded 4-digit
	// run (e.g. the compact "20260514").
	year := t.Year()
	if year == 0 {
		year = now.Year()
	}

	if hasTime {
		return time.Date(year, t.Month(), t.Day(), hour, minute, 0, 0, tz), nil
	}
	return time.Date(year, t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), tz), nil
}

func decodeAMPM(hStr, mStr, suffix string) (hour, minute int, ok bool) {
	if _, err := fmt.Sscanf(hStr, "%d", &hour); err != nil {
		return 0, 0, false
	}
	if mStr != "" {
		if _, err := fmt.Sscanf(mStr, "%d", &minute); err != nil {
			return 0, 0, false
		}
	}
	switch strings.ToLower(suffix) {
	case "am":
		if hour == 12 {
			hour = 0
		}
	case "pm":
		if hour != 12 {
			hour += 12
		}
	default:
		return 0, 0, false
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, false
	}
	return hour, minute, true
}

var timeOfDayRe = regexp.MustCompile(`\s+(\d{1,2})(?::(\d{2}))?\s*(am|pm)?$`)

func tryRelative(s string, tz *time.Location, now time.Time) (time.Time, bool) {
	if s == "now" {
		return now.In(tz), true
	}

	base, rest, found := splitRelativeKeyword(s)
	if !found {
		return time.Time{}, false
	}

	day := relativeDay(base, now, tz)

	if strings.TrimSpace(rest) == "" {
		return day, true
	}

	hour, min, ok := parseTimeOfDay(rest)
	if !ok {
		return time.Time{}, false
	}
	return time.Date(day.Year(), day.Month(), day.Day(), hour, min, 0, 0, tz), true
}

func splitRelativeKeyword(s string) (base, rest string, ok bool) {
	for _, kw := range []string{"today", "tomorrow", "yesterday"} {
		if s == kw {
			return kw, "", true
		}
		if strings.HasPrefix(s, kw+" ") {
			return kw, s[len(kw):], true
		}
	}
	return "", "", false
}

func relativeDay(kw string, now time.Time, tz *time.Location) time.Time {
	localNow := now.In(tz)
	y, m, d := localNow.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, tz)
	switch kw {
	case "today":
		return day
	case "tomorrow":
		return day.AddDate(0, 0, 1)
	case "yesterday":
		return day.AddDate(0, 0, -1)
	}
	return day
}

func parseTimeOfDay(s string) (hour, minute int, ok bool) {
	m := timeOfDayRe.FindStringSubmatch(" " + strings.TrimSpace(s))
	if m == nil {
		return 0, 0, false
	}
	var h, min int
	fmt.Sscanf(m[1], "%d", &h)
	if m[2] != "" {
		fmt.Sscanf(m[2], "%d", &min)
	}
	switch m[3] {
	case "am":
		if h == 12 {
			h = 0
		}
	case "pm":
		if h != 12 {
			h += 12
		}
	}
	if h < 0 || h > 23 || min < 0 || min > 59 {
		return 0, 0, false
	}
	return h, min, true
}
