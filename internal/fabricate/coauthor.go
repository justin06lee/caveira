package fabricate

import (
	"math/rand"
	"strings"
)

// Commit-type multipliers applied to a player's base model-co-author rate.
const (
	choreTypeFactor = 1.5 // documentation / chore commits — models show up more
	codeTypeFactor  = 1.0 // code / test commits — base rate
)

// InjectCoAuthors appends a single "Co-Authored-By:" trailer to plan commits,
// driven by the ModelReport. For each non-merge, non-empty commit it looks up
// the author's player profile; with probability min(1, Rate*typeFactor) it
// appends a model co-author chosen weighted by the player's model mix. It is a
// no-op when the report contains no models. All randomness uses rng, so seeded
// runs stay reproducible.
func InjectCoAuthors(plan *Plan, report *ModelReport, rng *rand.Rand) {
	if report == nil || len(report.Models) == 0 {
		return
	}
	for i := range plan.Commits {
		sc := &plan.Commits[i]
		if sc.IsMerge || len(sc.Added) == 0 {
			continue
		}
		prof, ok := report.Profiles[strings.ToLower(strings.TrimSpace(sc.Author.Email))]
		if !ok || prof.Rate <= 0 {
			continue
		}
		factor := codeTypeFactor
		if allChore(sc.Added) {
			factor = choreTypeFactor
		}
		p := prof.Rate * factor
		if p > 1.0 {
			p = 1.0
		}
		if rng.Float64() >= p {
			continue
		}
		model, ok := pickModel(report.Models, prof.Mix, rng)
		if !ok {
			continue
		}
		sc.Message = appendCoAuthor(sc.Message, model)
	}
}

// allChore reports whether every file in files classifies as Chore.
func allChore(files []FileRef) bool {
	for _, f := range files {
		if Classify(f.Path) != Chore {
			return false
		}
	}
	return len(files) > 0
}

// pickModel chooses one model from models, weighted by mix (keyed by lowercased
// model email). Returns false if the player's mix has no positive weight.
func pickModel(models []Identity, mix map[string]float64, rng *rand.Rand) (Identity, bool) {
	total := 0.0
	for _, m := range models {
		total += mix[strings.ToLower(strings.TrimSpace(m.Email))]
	}
	if total <= 0 {
		return Identity{}, false
	}
	r := rng.Float64() * total
	acc := 0.0
	for _, m := range models {
		acc += mix[strings.ToLower(strings.TrimSpace(m.Email))]
		if r < acc {
			return m, true
		}
	}
	return models[len(models)-1], true
}

// appendCoAuthor appends a Co-Authored-By git trailer to a commit message.
func appendCoAuthor(message string, model Identity) string {
	body := strings.TrimRight(message, "\n")
	return body + "\n\nCo-Authored-By: " + model.Name + " <" + model.Email + ">"
}
