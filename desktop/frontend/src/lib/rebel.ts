// What the window says while caveira works, and what an empty chat asks:
// caveira is the underground, and this is it at work. The verbs are the
// terminal client's (tui/internal/ui/verbs.go).

const verbs = [
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
];

let last = "";

// pickVerb draws a verb for the next stretch of work, never the one that
// was just showing.
export function pickVerb(): string {
  let v = last;
  while (v === last) v = verbs[Math.floor(Math.random() * verbs.length)];
  last = v;
  return v;
}

// What an empty chat asks, by the time of day.
const greetings: { from: number; lines: string[] }[] = [
  {
    from: 0,
    lines: [
      "The city's asleep. What are we breaking?",
      "Lights out. What's the op?",
      "Nobody's watching. What's the job?",
      "Under cover of dark. What's the move?",
      "Graveyard shift. What's the target?",
      "The guards are dozing. What do we take?",
    ],
  },
  {
    from: 5,
    lines: [
      "Dawn raid. What's the target?",
      "First light. What are we taking today?",
      "Sun's up, cell's awake. What's the job?",
      "Fresh coffee, fresh orders. What's the op?",
      "Morning briefing. What's on the board?",
    ],
  },
  {
    from: 12,
    lines: [
      "Broad daylight. Make it quick. What's the job?",
      "Afternoon op. What are we hitting?",
      "The guards are at lunch. What do we take?",
      "Midday shift. What's the target?",
      "Hiding in plain sight. What's the move?",
    ],
  },
  {
    from: 17,
    lines: [
      "Dusk falls. What are we plotting?",
      "Streetlights on. What's the move?",
      "The cell regroups. What's next?",
      "Shift change at the gate. What's the op?",
      "Sun's going down. What's the plan?",
    ],
  },
  {
    from: 22,
    lines: [
      "Moving at midnight. What's the job?",
      "The city's asleep. What are we breaking?",
      "Lights out. What's the op?",
      "Under cover of dark. What's the move?",
    ],
  },
];

// greeting is what an empty chat asks at this hour. The seed keeps one
// chat's line the same while it is open.
export function greeting(seed: string, now = new Date()): string {
  const h = now.getHours();
  const { lines } = [...greetings].reverse().find((g) => h >= g.from)!;
  let n = 0;
  for (const ch of seed) n = (n * 31 + ch.charCodeAt(0)) >>> 0;
  return lines[n % lines.length];
}
