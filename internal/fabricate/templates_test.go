package fabricate

import (
	"math/rand"
	"strings"
	"testing"
)

func TestChoreMessage(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	got := ChoreMessage(rng)
	if !strings.HasPrefix(got, "chore:") {
		t.Errorf("chore msg = %q", got)
	}
}

func TestCodeMessage(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	got := CodeMessage("walk", rng)
	if !strings.HasPrefix(got, "feat(walk):") {
		t.Errorf("code msg = %q", got)
	}
}

func TestTestMessage(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	got := TestMessage("walk", rng)
	if !strings.HasPrefix(got, "test(walk):") {
		t.Errorf("test msg = %q", got)
	}
}

func TestMessage_DeterministicWithSeed(t *testing.T) {
	a := rand.New(rand.NewSource(7))
	b := rand.New(rand.NewSource(7))
	if CodeMessage("walk", a) != CodeMessage("walk", b) {
		t.Errorf("same seed produced different messages")
	}
}
