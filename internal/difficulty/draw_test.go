package difficulty

import (
	"math/rand"
	"testing"
)

func TestDraw_InRange(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 1000; i++ {
		for d := Trivial; d <= Substantial; d++ {
			got := Draw(d, rng)
			min := d.Base() - d.Deviation()
			max := d.Base() + d.Deviation()
			if got < min || got > max {
				t.Fatalf("Draw(%v) = %d, expected in [%d,%d]", d, got, min, max)
			}
		}
	}
}

func TestDraw_Deterministic(t *testing.T) {
	a := rand.New(rand.NewSource(99))
	b := rand.New(rand.NewSource(99))
	for i := 0; i < 50; i++ {
		x := Draw(Medium, a)
		y := Draw(Medium, b)
		if x != y {
			t.Fatalf("same seed produced different draws at i=%d: %d vs %d", i, x, y)
		}
	}
}
