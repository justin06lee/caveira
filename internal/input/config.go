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
	return nil
}

// WindowSize returns the duration between Start and End.
func (c *Config) WindowSize() time.Duration {
	return c.End.Sub(c.Start)
}
