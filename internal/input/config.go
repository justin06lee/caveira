package input

import (
	"errors"
	"time"
)

// Config is the parsed and validated CLI configuration.
type Config struct {
	Repo          string
	Start         time.Time
	End           time.Time
	Seed          int64
	HasSeed       bool
	DryRun        bool
	Push          bool
	PushProtected bool
	WindowTZ      *time.Location
	OutDir        string
	Preserve      bool // --preserve: keep every commit, scaling spacing down to fit instead of merging

	// Fabricate-mode fields
	Fabricate     bool
	PigsN         int      // 0 = not set
	RatsN         int      // 0 = not set
	PigIdentities []string // raw strings from --pig flags, parsed in fabricate.ParseIdentity
	RatIdentities []string // raw strings from --rat flags
	Pick          bool     // --pick: always open the interactive player picker
	Earned        bool     // --earned: weight author assignment by real commit-count distribution
}

// Validate returns an error if the configuration is unusable.
func (c *Config) Validate() error {
	if c.Repo == "" {
		return errors.New("--repo is required")
	}
	if c.WindowTZ == nil {
		return errors.New("--window-tz must resolve to a location")
	}
	if !c.Start.Before(c.End) {
		return errors.New("--start must be strictly before --end")
	}
	if c.PushProtected && !c.Push {
		return errors.New("--push-protected has no effect without --push")
	}

	fabFlagsUsed := c.PigsN > 0 || c.RatsN > 0 ||
		len(c.PigIdentities) > 0 || len(c.RatIdentities) > 0 || c.Pick || c.Earned
	if fabFlagsUsed && !c.Fabricate {
		return errors.New("--pigs, --rats, --pig, --rat, --pick, --earned all require --fabricate")
	}

	if c.PigsN > 0 && c.RatsN > 0 {
		return errors.New("--pigs and --rats are mutually exclusive")
	}
	if len(c.PigIdentities) > 0 && c.PigsN == 0 {
		return errors.New("--pig requires --pigs N")
	}
	if len(c.RatIdentities) > 0 && c.RatsN == 0 {
		return errors.New("--rat requires --rats N")
	}
	if c.PigsN < 0 {
		return errors.New("--pigs must be >= 1")
	}
	if c.RatsN < 0 {
		return errors.New("--rats must be >= 1")
	}
	if c.Pick && !c.Fabricate {
		return errors.New("--pick requires --fabricate")
	}
	if c.Pick && c.PigsN == 0 && c.RatsN == 0 {
		return errors.New("--pick requires --pigs N or --rats N")
	}
	if c.Earned && !c.Fabricate {
		return errors.New("--earned requires --fabricate")
	}
	if c.Earned && c.PigsN == 0 && c.RatsN == 0 {
		return errors.New("--earned requires --pigs N or --rats N")
	}
	return nil
}

// WindowSize returns the duration between Start and End.
func (c *Config) WindowSize() time.Duration {
	return c.End.Sub(c.Start)
}
