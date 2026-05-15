package fabricate

import (
	"fmt"
	"math/rand"
	"path"
)

var (
	choreVariants = []string{
		"chore: project scaffolding",
		"chore: initial scaffolding",
	}
	codeVerbs = []string{"add", "introduce", "scaffold"}
	testVerbs = []string{"add tests for", "tests for"}
)

// ChoreMessage returns a chore commit message.
func ChoreMessage(rng *rand.Rand) string {
	return choreVariants[rng.Intn(len(choreVariants))]
}

// CodeMessage returns "feat(<name>): <verb> <name>" where name = basename(dir).
func CodeMessage(dir string, rng *rand.Rand) string {
	name := basenameDir(dir)
	verb := codeVerbs[rng.Intn(len(codeVerbs))]
	return fmt.Sprintf("feat(%s): %s %s", name, verb, name)
}

// TestMessage returns "test(<name>): <verb> <name>".
func TestMessage(dir string, rng *rand.Rand) string {
	name := basenameDir(dir)
	verb := testVerbs[rng.Intn(len(testVerbs))]
	return fmt.Sprintf("test(%s): %s %s", name, verb, name)
}

func basenameDir(dir string) string {
	if dir == "." {
		return "root"
	}
	return path.Base(dir)
}
