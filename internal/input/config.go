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

	// Fabricate-mode fields (Phase 1)
	Fabricate     bool
	Flurry        bool
	PigsN         int      // 0 = not set
	RatsN         int      // 0 = not set
	PigIdentities []string // raw strings from --pig flags, parsed in fabricate.ParseIdentity
	RatIdentities []string // raw strings from --rat flags

	// Fabricate-mode fields (Phase 2: LLM providers)
	Provider   string        // "" = no LLM engine; else one of the registry names
	Model      string        // optional model override for the LLM provider
	LLMTimeout time.Duration // per-LLM-call timeout; 0 = use default
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

	fabFlagsUsed := c.Flurry || c.Provider != "" || c.PigsN > 0 || c.RatsN > 0 ||
		len(c.PigIdentities) > 0 || len(c.RatIdentities) > 0
	if fabFlagsUsed && !c.Fabricate {
		return errors.New("--flurry, --pigs, --rats, --pig, --rat all require --fabricate")
	}

	baseEngines := 0
	if c.Flurry {
		baseEngines++
	}
	if c.Provider != "" {
		baseEngines++
	}
	if c.Fabricate && baseEngines == 0 {
		return errors.New("--fabricate requires a base engine: --flurry, --groq, --claude-code, --codex, --nvidia, or --opencode")
	}
	if baseEngines > 1 {
		return errors.New("base engines are mutually exclusive: pick one of --flurry, --groq, --claude-code, --codex, --nvidia, --opencode")
	}
	if (c.Model != "" || c.LLMTimeout != 0) && c.Provider == "" {
		return errors.New("--model and --llm-timeout require an LLM engine (--groq, --claude-code, --codex, --nvidia, or --opencode)")
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
	return nil
}

// WindowSize returns the duration between Start and End.
func (c *Config) WindowSize() time.Duration {
	return c.End.Sub(c.Start)
}
