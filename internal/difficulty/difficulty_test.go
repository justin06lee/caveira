package difficulty

import "testing"

func TestScore(t *testing.T) {
	cases := []struct {
		lines, files, newFiles, want int
	}{
		{0, 0, 0, 0},
		{5, 1, 0, 15},
		{20, 3, 1, 75},
		{200, 5, 2, 300},
	}
	for _, c := range cases {
		got := Score(c.lines, c.files, c.newFiles)
		if got != c.want {
			t.Errorf("Score(%d,%d,%d) = %d, want %d", c.lines, c.files, c.newFiles, got, c.want)
		}
	}
}

func TestBucket(t *testing.T) {
	cases := []struct {
		score   int
		isMerge bool
		want    Difficulty
	}{
		{0, false, Trivial},
		{10, false, Trivial},
		{11, false, Easy},
		{50, false, Easy},
		{51, false, Medium},
		{200, false, Medium},
		{201, false, Hard},
		{600, false, Hard},
		{601, false, Substantial},
		{9999, false, Substantial},
		{9999, true, Trivial}, // merge override
	}
	for _, c := range cases {
		got := Bucket(c.score, c.isMerge)
		if got != c.want {
			t.Errorf("Bucket(%d, merge=%v) = %v, want %v", c.score, c.isMerge, got, c.want)
		}
	}
}

func TestBaseAndDeviation(t *testing.T) {
	cases := []struct {
		d    Difficulty
		base int
		dev  int
	}{
		{Trivial, 5, 3},
		{Easy, 15, 7},
		{Medium, 30, 13},
		{Hard, 60, 17},
		{Substantial, 90, 23},
	}
	for _, c := range cases {
		if c.d.Base() != c.base {
			t.Errorf("%v.Base() = %d, want %d", c.d, c.d.Base(), c.base)
		}
		if c.d.Deviation() != c.dev {
			t.Errorf("%v.Deviation() = %d, want %d", c.d, c.d.Deviation(), c.dev)
		}
	}
}
