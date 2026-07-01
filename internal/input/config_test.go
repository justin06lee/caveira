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

func TestConfig_Validate_PushProtectedRequiresPush(t *testing.T) {
	tz := time.UTC
	base := func() Config {
		return Config{
			Repo:     "/tmp/x",
			Start:    time.Date(2026, 5, 14, 12, 0, 0, 0, tz),
			End:      time.Date(2026, 5, 14, 13, 0, 0, 0, tz),
			WindowTZ: tz,
		}
	}

	c := base()
	c.PushProtected = true // no --push
	if err := c.Validate(); err == nil {
		t.Fatal("expected error: --push-protected without --push")
	}

	c = base()
	c.Push = true
	c.PushProtected = true
	if err := c.Validate(); err != nil {
		t.Fatalf("--push + --push-protected should be valid, got %v", err)
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
		{"pigs without fabricate", func(c *Config) { c.PigsN = 2 }, "require --fabricate"},
		{"rats without fabricate", func(c *Config) { c.RatsN = 2 }, "require --fabricate"},
		{"pig without pigs", func(c *Config) { c.Fabricate = true; c.PigIdentities = []string{"x"} }, "--pig requires --pigs"},
		{"rat without rats", func(c *Config) { c.Fabricate = true; c.RatIdentities = []string{"x"} }, "--rat requires --rats"},
		{"pigs and rats together", func(c *Config) { c.Fabricate = true; c.PigsN = 2; c.RatsN = 2 }, "mutually exclusive"},
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

func TestConfig_Validate_BareFabricateOK(t *testing.T) {
	tz := time.UTC
	c := Config{
		Repo: "/tmp/x", WindowTZ: tz,
		Start:     time.Date(2026, 5, 14, 12, 0, 0, 0, tz),
		End:       time.Date(2026, 5, 14, 13, 0, 0, 0, tz),
		Fabricate: true,
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected bare --fabricate to validate, got %v", err)
	}
}

func TestConfig_Validate_FabricateOK(t *testing.T) {
	tz := time.UTC
	c := Config{
		Repo: "/tmp/x", WindowTZ: tz,
		Start:     time.Date(2026, 5, 14, 12, 0, 0, 0, tz),
		End:       time.Date(2026, 5, 14, 13, 0, 0, 0, tz),
		Fabricate: true, PigsN: 2,
		PigIdentities: []string{"Alice <a@x.com>", "Bob <b@x.com>"},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func baseValidConfig() *Config {
	return &Config{
		Repo:     "/tmp/x",
		Start:    time.Date(2026, 5, 14, 13, 0, 0, 0, time.UTC),
		End:      time.Date(2026, 5, 14, 17, 0, 0, 0, time.UTC),
		WindowTZ: time.UTC,
	}
}

func TestValidate_PickRequiresPigsOrRats(t *testing.T) {
	c := baseValidConfig()
	c.Fabricate = true
	c.Pick = true
	if err := c.Validate(); err == nil {
		t.Fatal("expected --pick without --pigs/--rats to be rejected")
	}

	c.RatsN = 3
	if err := c.Validate(); err != nil {
		t.Fatalf("--pick with --rats should validate, got: %v", err)
	}
}

func TestValidate_PickRequiresFabricate(t *testing.T) {
	c := baseValidConfig()
	c.Pick = true
	c.RatsN = 3
	if err := c.Validate(); err == nil {
		t.Fatal("expected --pick without --fabricate to be rejected")
	}
}

func TestValidate_EarnedRequiresPigsOrRats(t *testing.T) {
	c := baseValidConfig()
	c.Fabricate = true
	c.Earned = true
	if err := c.Validate(); err == nil {
		t.Fatal("expected --earned without --pigs/--rats to be rejected")
	}
	c.PigsN = 2
	if err := c.Validate(); err != nil {
		t.Fatalf("--earned with --pigs should validate, got: %v", err)
	}
}

func TestValidate_EarnedRequiresFabricate(t *testing.T) {
	c := baseValidConfig()
	c.Earned = true
	c.PigsN = 2
	if err := c.Validate(); err == nil {
		t.Fatal("expected --earned without --fabricate to be rejected")
	}
}

func TestValidate_PreserveHasNoDependencies(t *testing.T) {
	// --preserve is a scheduling concern: it is valid on its own (retime mode)
	// and alongside fabricate flags, with no gating like --pick/--earned.
	c := baseValidConfig()
	c.Preserve = true
	if err := c.Validate(); err != nil {
		t.Fatalf("--preserve alone should validate, got: %v", err)
	}

	c = baseValidConfig()
	c.Preserve = true
	c.Fabricate = true
	c.PigsN = 2
	if err := c.Validate(); err != nil {
		t.Fatalf("--preserve with --fabricate --pigs should validate, got: %v", err)
	}
}

func TestValidate_LeechesValidInRetime(t *testing.T) {
	c := baseValidConfig()
	c.LeechesN = 3
	c.LeechIdentities = []string{"Alice <a@x.com>"}
	if err := c.Validate(); err != nil {
		t.Fatalf("--leeches in retime mode should validate, got: %v", err)
	}
}

func TestValidate_LeechesRejectsFabricate(t *testing.T) {
	c := baseValidConfig()
	c.Fabricate = true
	c.LeechesN = 2
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "--fabricate") {
		t.Fatalf("expected --leeches+--fabricate to be rejected, got: %v", err)
	}
}

func TestValidate_LeechRequiresLeeches(t *testing.T) {
	c := baseValidConfig()
	c.LeechIdentities = []string{"Alice <a@x.com>"}
	if err := c.Validate(); err == nil {
		t.Fatal("expected --leech without --leeches to be rejected")
	}
}

func TestValidate_PickAllowedWithLeeches(t *testing.T) {
	c := baseValidConfig()
	c.Pick = true
	c.LeechesN = 2
	if err := c.Validate(); err != nil {
		t.Fatalf("--pick with --leeches should validate, got: %v", err)
	}
}
