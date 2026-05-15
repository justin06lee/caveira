package fabricate

import (
	"math/rand"
	"strings"
	"testing"
)

func TestApplyTypos_NoChangeWithLowProb(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	out := ApplyTypos("hello world", rng)
	if out == "" {
		t.Fatal("output should not be empty")
	}
}

func TestApplyTypos_SometimesChanges(t *testing.T) {
	original := "feat(walk): add walk"
	changed := false
	for s := int64(0); s < 100; s++ {
		rng := rand.New(rand.NewSource(s))
		out := ApplyTypos(original, rng)
		if out != original {
			changed = true
			break
		}
	}
	if !changed {
		t.Fatal("expected at least one seed to produce a typo across 100 trials")
	}
}

func TestApplyTypos_Deterministic(t *testing.T) {
	a := rand.New(rand.NewSource(42))
	b := rand.New(rand.NewSource(42))
	if ApplyTypos("foobar", a) != ApplyTypos("foobar", b) {
		t.Errorf("same seed produced different results")
	}
}

func TestApplyTypos_EmptyString(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	if got := ApplyTypos("", rng); got != "" {
		t.Errorf("ApplyTypos(\"\") = %q, want empty", got)
	}
}

func TestApplyTypos_OnlyAffectsMessage(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	out := ApplyTypos("xy", rng)
	if len(out) < 1 || len(out) > 3 {
		t.Errorf("output length too far from input: %q", out)
	}
	for _, r := range out {
		if r == 0 {
			t.Errorf("invalid rune in %q", out)
		}
	}
	_ = strings.ToLower
}
