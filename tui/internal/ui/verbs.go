package ui

import "math/rand/v2"

// rebelVerbs are what the status line says while the model thinks or
// writes: caveira is the underground, and this is it at work. Tool calls
// say what they actually run instead. The status line adds the ellipsis.
var rebelVerbs = []string{
	"Plotting",
	"Scheming",
	"Conspiring",
	"Skulking",
	"Lurking",
	"Sabotaging",
	"Smuggling",
	"Bootlegging",
	"Infiltrating",
	"Ambushing",
	"Flanking",
	"Scavenging",
	"Jury-rigging",
	"Hotwiring",
	"Fighting dirty",
	"Throwing pocket sand",
	"Going dark",
	"Going underground",
	"Laying low",
	"Casing the joint",
	"Digging a tunnel",
	"Cutting the wires",
	"Picking the lock",
	"Cracking the safe",
	"Jamming the signal",
	"Tapping the line",
	"Forging papers",
	"Printing pamphlets",
	"Tagging the walls",
	"Rallying the cell",
	"Whispering in code",
	"Making a dead drop",
	"Running contraband",
	"Greasing palms",
	"Bribing the guards",
	"Slipping the tail",
	"Covering tracks",
	"Dodging searchlights",
	"Dodging the censors",
	"Mapping the sewers",
	"Moving at midnight",
	"Waiting for the signal",
	"Lighting the fuse",
	"Setting the trap",
	"Rigging the tripwire",
	"Kicking in the door",
	"Breaking the chains",
	"Torching the rulebook",
	"Ignoring orders",
	"Hijacking the broadcast",
	"Stealing the blueprints",
	"Raiding the depot",
	"Fencing the loot",
	"Stashing the goods",
	"Swapping the plates",
	"Burning the evidence",
	"Sharpening the shiv",
	"Loading the slingshot",
	"Unscrewing the cameras",
	"Pulling a fast one",
	"Holding the line",
	"Raising the black flag",
}

// pickVerb draws a verb for the next stretch of work, never the one that
// was just showing.
func pickVerb(prev string) string {
	for {
		v := rebelVerbs[rand.IntN(len(rebelVerbs))]
		if v != prev {
			return v
		}
	}
}
