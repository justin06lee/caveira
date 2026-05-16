package fabricate

import (
	"math/rand"
	"testing"
)

func TestPickAuthor_UniformCoversAll(t *testing.T) {
	ids := []Identity{
		{Name: "A", Email: "a@x"}, {Name: "B", Email: "b@x"}, {Name: "C", Email: "c@x"},
	}
	rng := rand.New(rand.NewSource(1))
	seen := map[string]int{}
	for i := 0; i < 3000; i++ {
		seen[pickAuthor(ids, nil, rng).Email]++
	}
	for _, id := range ids {
		if seen[id.Email] == 0 {
			t.Fatalf("uniform draw never picked %s", id.Email)
		}
	}
}

func TestPickAuthor_Weighted(t *testing.T) {
	ids := []Identity{{Name: "Heavy", Email: "h@x"}, {Name: "Light", Email: "l@x"}}
	rng := rand.New(rand.NewSource(1))
	seen := map[string]int{}
	for i := 0; i < 5000; i++ {
		seen[pickAuthor(ids, []int{9, 1}, rng).Email]++
	}
	if seen["h@x"] <= seen["l@x"] {
		t.Fatalf("weighted draw not skewed: h=%d l=%d", seen["h@x"], seen["l@x"])
	}
	if seen["h@x"] < 3500 || seen["l@x"] == 0 {
		t.Fatalf("weighted distribution off: h=%d l=%d (want h~4500, l~500)", seen["h@x"], seen["l@x"])
	}
}

func TestPickAuthor_AllZeroWeightsUniform(t *testing.T) {
	ids := []Identity{{Name: "A", Email: "a@x"}, {Name: "B", Email: "b@x"}}
	rng := rand.New(rand.NewSource(1))
	seen := map[string]int{}
	for i := 0; i < 2000; i++ {
		seen[pickAuthor(ids, []int{0, 0}, rng).Email]++
	}
	if seen["a@x"] == 0 || seen["b@x"] == 0 {
		t.Fatalf("all-zero weights should fall back to uniform: %+v", seen)
	}
}

func TestPickAuthor_MismatchedWeightsUniform(t *testing.T) {
	ids := []Identity{{Name: "A", Email: "a@x"}, {Name: "B", Email: "b@x"}}
	rng := rand.New(rand.NewSource(1))
	seen := map[string]int{}
	for i := 0; i < 2000; i++ {
		seen[pickAuthor(ids, []int{5}, rng).Email]++
	}
	if seen["a@x"] == 0 || seen["b@x"] == 0 {
		t.Fatalf("mismatched-length weights should fall back to uniform: %+v", seen)
	}
}

func TestPickAuthor_SingleIdentity(t *testing.T) {
	ids := []Identity{{Name: "Only", Email: "only@x"}}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 10; i++ {
		if pickAuthor(ids, nil, rng).Email != "only@x" {
			t.Fatal("single-identity pick must always return that identity")
		}
	}
}
