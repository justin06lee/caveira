package input

import (
	"testing"
	"time"
)

func TestConfig_Validate_StartBeforeEnd(t *testing.T) {
	tz := time.UTC
	c := Config{
		Repo:     "/tmp/x",
		Start:    time.Date(2026, 5, 14, 13, 0, 0, 0, tz),
		End:      time.Date(2026, 5, 14, 12, 0, 0, 0, tz),
		WindowTZ: tz,
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected validation error when start >= end")
	}
}

func TestConfig_Validate_RepoRequired(t *testing.T) {
	tz := time.UTC
	c := Config{
		Start:    time.Date(2026, 5, 14, 12, 0, 0, 0, tz),
		End:      time.Date(2026, 5, 14, 13, 0, 0, 0, tz),
		WindowTZ: tz,
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected validation error when Repo empty")
	}
}

func TestConfig_Validate_OK(t *testing.T) {
	tz := time.UTC
	c := Config{
		Repo:     "/tmp/x",
		Start:    time.Date(2026, 5, 14, 12, 0, 0, 0, tz),
		End:      time.Date(2026, 5, 14, 13, 0, 0, 0, tz),
		WindowTZ: tz,
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}
