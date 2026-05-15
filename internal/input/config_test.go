package input

import (
	"strings"
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

func TestConfig_Validate_FabricateFlagDependencies(t *testing.T) {
	tz := time.UTC
	base := Config{
		Repo:     "/tmp/x",
		Start:    time.Date(2026, 5, 14, 12, 0, 0, 0, tz),
		End:      time.Date(2026, 5, 14, 13, 0, 0, 0, tz),
		WindowTZ: tz,
	}
	cases := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"flurry without fabricate", func(c *Config) { c.Flurry = true }, "require --fabricate"},
		{"pigs without fabricate", func(c *Config) { c.PigsN = 2 }, "require --fabricate"},
		{"rats without fabricate", func(c *Config) { c.RatsN = 2 }, "require --fabricate"},
		{"pig without pigs", func(c *Config) { c.Fabricate = true; c.Flurry = true; c.PigIdentities = []string{"x"} }, "--pig requires --pigs"},
		{"rat without rats", func(c *Config) { c.Fabricate = true; c.Flurry = true; c.RatIdentities = []string{"x"} }, "--rat requires --rats"},
		{"pigs and rats together", func(c *Config) { c.Fabricate = true; c.Flurry = true; c.PigsN = 2; c.RatsN = 2 }, "mutually exclusive"},
		{"fabricate without flurry", func(c *Config) { c.Fabricate = true }, "requires --flurry"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mutate(&c)
			err := c.Validate()
			if err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("expected %q in error, got: %v", tc.want, err)
			}
		})
	}
}

func TestConfig_Validate_FabricateOK(t *testing.T) {
	tz := time.UTC
	c := Config{
		Repo: "/tmp/x", WindowTZ: tz,
		Start:     time.Date(2026, 5, 14, 12, 0, 0, 0, tz),
		End:       time.Date(2026, 5, 14, 13, 0, 0, 0, tz),
		Fabricate: true, Flurry: true, PigsN: 2,
		PigIdentities: []string{"Alice <a@x.com>", "Bob <b@x.com>"},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}
