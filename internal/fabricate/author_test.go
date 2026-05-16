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

func TestEarnedWeights(t *testing.T) {
	discovered := []DiscoveredIdentity{
		{Identity: Identity{Name: "Heavy", Email: "h@x"}, Commits: 40},
		{Identity: Identity{Name: "Light", Email: "l@x"}, Commits: 10},
	}
	ids := []Identity{
		{Name: "Heavy", Email: "h@x"},
		{Name: "Light", Email: "l@x"},
		{Name: "Newcomer", Email: "new@x"},
	}
	w := EarnedWeights(ids, discovered, nil)
	if w == nil || len(w) != 3 {
		t.Fatalf("expected 3 weights, got %+v", w)
	}
	if w[0] != 40 || w[1] != 10 {
		t.Fatalf("discovered weights wrong: %+v", w)
	}
	if w[2] != 25 {
		t.Fatalf("non-discovered weight should be the mean (25), got %d", w[2])
	}
}

func TestEarnedWeights_NoDiscoveredIsNil(t *testing.T) {
	ids := []Identity{{Name: "A", Email: "a@x"}}
	if w := EarnedWeights(ids, nil, nil); w != nil {
		t.Fatalf("no discovered identities should yield nil weights, got %+v", w)
	}
}

func TestEarnedWeights_MailmapCanonicalized(t *testing.T) {
	discovered := []DiscoveredIdentity{
		{Identity: Identity{Name: "Jay", Email: "jay@personal.com"}, Commits: 30},
	}
	mm := ParseMailmap([]byte("Jay <jay@personal.com> <jay@work.com>\n"))
	ids := []Identity{{Name: "Jay", Email: "jay@work.com"}}
	w := EarnedWeights(ids, discovered, mm)
	if len(w) != 1 || w[0] != 30 {
		t.Fatalf("mailmap-canonicalized weight wrong: %+v", w)
	}
}
