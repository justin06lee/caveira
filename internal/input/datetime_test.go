package input

import (
	"testing"
	"time"
)

func TestParseDateTime_Absolute(t *testing.T) {
	tz, _ := time.LoadLocation("America/New_York")
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, tz)

	cases := []struct {
		in   string
		want time.Time
	}{
		{"2026-05-14 13:00", time.Date(2026, 5, 14, 13, 0, 0, 0, tz)},
		{"2026-05-14 1pm", time.Date(2026, 5, 14, 13, 0, 0, 0, tz)},
		{"5/14 1pm", time.Date(2026, 5, 14, 13, 0, 0, 0, tz)},
		{"May 14 1pm", time.Date(2026, 5, 14, 13, 0, 0, 0, tz)},
	}
	for _, c := range cases {
		got, err := ParseDateTime(c.in, tz, now)
		if err != nil {
			t.Errorf("ParseDateTime(%q) error: %v", c.in, err)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("ParseDateTime(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseDateTime_Relative(t *testing.T) {
	tz, _ := time.LoadLocation("America/New_York")
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, tz)

	cases := []struct {
		in   string
		want time.Time
	}{
		{"now", now},
		{"today", time.Date(2026, 5, 14, 0, 0, 0, 0, tz)},
		{"tomorrow 5pm", time.Date(2026, 5, 15, 17, 0, 0, 0, tz)},
		{"yesterday 9am", time.Date(2026, 5, 13, 9, 0, 0, 0, tz)},
	}
	for _, c := range cases {
		got, err := ParseDateTime(c.in, tz, now)
		if err != nil {
			t.Errorf("ParseDateTime(%q) error: %v", c.in, err)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("ParseDateTime(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseDateTime_Unparseable(t *testing.T) {
	tz := time.UTC
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, tz)
	_, err := ParseDateTime("absolute gibberish", tz, now)
	if err == nil {
		t.Fatal("expected error on gibberish input")
	}
}
