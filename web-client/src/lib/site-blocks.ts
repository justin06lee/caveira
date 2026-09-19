// The block vocabulary the site editor works in.
//
// A layout is just an ordered list of blocks, each a type plus a bag of props.
// `site-layout.json` at the repo root holds the current one; the editor writes
// it and the renderer in components/site/block-renderer.tsx draws it. Keeping
// the schema here means the editor's inspector is generated, not hand-written:
// add a field to FIELDS and it shows up in the UI.

export type BlockType =
  | "hero"
  | "marquee"
  | "statement"
  | "features"
  | "split"
  | "chips"
  | "steps"
  | "pricing"
  | "quotes"
  | "faq"
  | "cta";

export type PropValue = string | boolean | string[] | Record<string, string>[];
export type BlockProps = Record<string, PropValue>;

export type Block = {
  id: string;
  type: BlockType;
  props: BlockProps;
};

export type SiteLayout = {
  version: 1;
  updatedAt?: string;
  blocks: Block[];
};

/* ------------------------------------------------------------------ */
/* field schema — drives the inspector                                  */
/* ------------------------------------------------------------------ */

export type Field =
  | { kind: "text"; key: string; label: string; hint?: string }
  | { kind: "textarea"; key: string; label: string; hint?: string; rows?: number }
  | { kind: "toggle"; key: string; label: string; hint?: string }
  | {
      kind: "select";
      key: string;
      label: string;
      options: { value: string; label: string }[];
      hint?: string;
    }
  /** A list of plain strings. */
  | { kind: "strings"; key: string; label: string; hint?: string; itemLabel?: string }
  /** A list of objects, each edited with the given subfields. */
  | {
      kind: "items";
      key: string;
      label: string;
      hint?: string;
      itemLabel?: string;
      subfields: { key: string; label: string; multiline?: boolean }[];
    };

const BAND: Field = {
  kind: "toggle",
  key: "band",
  label: "Raised band",
  hint: "Slightly lighter background with rules above and below, for rhythm.",
};

const LINK_FIELDS: Field[] = [
  { kind: "text", key: "primaryLabel", label: "Primary button" },
  { kind: "text", key: "primaryHref", label: "Primary link" },
  { kind: "text", key: "secondaryLabel", label: "Secondary button" },
  { kind: "text", key: "secondaryHref", label: "Secondary link" },
];

export const FIELDS: Record<BlockType, Field[]> = {
  hero: [
    { kind: "text", key: "chip", label: "Chip", hint: "Small pill above the headline. Empty to hide." },
    {
      kind: "textarea",
      key: "headline",
      label: "Headline",
      rows: 3,
      hint: "One line per line break.",
    },
    { kind: "text", key: "accent", label: "Accent line", hint: "Final line, in ember red." },
    { kind: "textarea", key: "body", label: "Body", rows: 4 },
    ...LINK_FIELDS,
    { kind: "text", key: "terminalNote", label: "Terminal strip", hint: "Mono line under the buttons. Empty to hide." },
    {
      kind: "select",
      key: "shader",
      label: "Background",
      options: [
        { value: "cycle", label: "ascii — drifting (all three)" },
        { value: "plasma", label: "ascii — plasma" },
        { value: "tunnel", label: "ascii — tunnel" },
        { value: "ripple", label: "ascii — ripple" },
        { value: "none", label: "grid only" },
      ],
    },
    {
      kind: "select",
      key: "align",
      label: "Alignment",
      options: [
        { value: "left", label: "left" },
        { value: "center", label: "centered" },
      ],
    },
  ],
  marquee: [
    { kind: "strings", key: "items", label: "Ticker items", itemLabel: "item" },
  ],
  statement: [
    { kind: "textarea", key: "lead", label: "Lead", rows: 3, hint: "Bright half of the sentence." },
    { kind: "textarea", key: "tail", label: "Tail", rows: 4, hint: "Dimmer continuation." },
    BAND,
  ],
  features: [
    { kind: "text", key: "eyebrow", label: "Eyebrow" },
    { kind: "text", key: "title", label: "Title" },
    { kind: "textarea", key: "lede", label: "Lede", rows: 3 },
    {
      kind: "select",
      key: "columns",
      label: "Columns",
      options: [
        { value: "2", label: "2 across" },
        { value: "3", label: "3 across" },
        { value: "4", label: "4 across" },
      ],
    },
    { kind: "toggle", key: "numbered", label: "Ghost numbers" },
    {
      kind: "items",
      key: "cards",
      label: "Cards",
      itemLabel: "card",
      subfields: [
        { key: "title", label: "Title" },
        { key: "body", label: "Body", multiline: true },
      ],
    },
    BAND,
  ],
  split: [
    { kind: "text", key: "eyebrow", label: "Eyebrow" },
    { kind: "text", key: "title", label: "Title" },
    { kind: "textarea", key: "body", label: "Body", rows: 5 },
    ...LINK_FIELDS,
    {
      kind: "select",
      key: "media",
      label: "Right side",
      options: [
        { value: "terminal", label: "terminal window" },
        { value: "shader", label: "ascii panel" },
        { value: "none", label: "nothing" },
      ],
    },
    { kind: "text", key: "terminalTitle", label: "Terminal title" },
    {
      kind: "strings",
      key: "terminalLines",
      label: "Terminal lines",
      itemLabel: "line",
      hint: "Start a line with “+ ” for green, “! ” for red, “- ” for grey. Blank line for a gap.",
    },
    { kind: "toggle", key: "reverse", label: "Media on the left" },
    BAND,
  ],
  chips: [
    { kind: "text", key: "eyebrow", label: "Eyebrow" },
    { kind: "text", key: "title", label: "Title" },
    { kind: "textarea", key: "lede", label: "Lede", rows: 3 },
    { kind: "strings", key: "items", label: "Chips", itemLabel: "chip" },
    { kind: "text", key: "note", label: "Footnote", hint: "Mono comment under the chips." },
    BAND,
  ],
  steps: [
    { kind: "text", key: "eyebrow", label: "Eyebrow" },
    { kind: "text", key: "title", label: "Title" },
    { kind: "textarea", key: "lede", label: "Lede", rows: 3 },
    {
      kind: "items",
      key: "items",
      label: "Steps",
      itemLabel: "step",
      subfields: [
        { key: "title", label: "Title" },
        { key: "body", label: "Body", multiline: true },
      ],
    },
    BAND,
  ],
  pricing: [
    { kind: "text", key: "eyebrow", label: "Eyebrow" },
    { kind: "text", key: "title", label: "Title" },
    { kind: "textarea", key: "lede", label: "Lede", rows: 3 },
    {
      kind: "toggle",
      key: "live",
      label: "Use the real plans",
      hint: "Reads src/lib/plans.ts. Turn off to write your own cards below.",
    },
    {
      kind: "items",
      key: "cards",
      label: "Custom plans",
      itemLabel: "plan",
      subfields: [
        { key: "name", label: "Name" },
        { key: "price", label: "Price" },
        { key: "tagline", label: "Tagline" },
        { key: "features", label: "Features (one per line)", multiline: true },
      ],
    },
    BAND,
  ],
  quotes: [
    { kind: "text", key: "eyebrow", label: "Eyebrow" },
    { kind: "text", key: "title", label: "Title" },
    {
      kind: "items",
      key: "items",
      label: "Quotes",
      itemLabel: "quote",
      subfields: [
        { key: "quote", label: "Quote", multiline: true },
        { key: "name", label: "Name" },
        { key: "role", label: "Role" },
      ],
    },
    BAND,
  ],
  faq: [
    { kind: "text", key: "eyebrow", label: "Eyebrow" },
    { kind: "text", key: "title", label: "Title" },
    {
      kind: "items",
      key: "items",
      label: "Questions",
      itemLabel: "question",
      subfields: [
        { key: "q", label: "Question" },
        { key: "a", label: "Answer", multiline: true },
      ],
    },
    BAND,
  ],
  cta: [
    { kind: "textarea", key: "headline", label: "Headline", rows: 2 },
    { kind: "text", key: "accent", label: "Accent line" },
    { kind: "textarea", key: "body", label: "Body", rows: 3 },
    ...LINK_FIELDS,
    { kind: "toggle", key: "glow", label: "Ember glow" },
  ],
};

/* ------------------------------------------------------------------ */
/* the palette — label, blurb and starting props for each block         */
/* ------------------------------------------------------------------ */

export const BLOCK_META: Record<
  BlockType,
  { label: string; blurb: string; defaults: BlockProps }
> = {
  hero: {
    label: "Hero",
    blurb: "Headline, buttons, ascii background.",
    defaults: {
      chip: "abliterated · uncensored · local",
      headline: "the coding agent\nfor models that",
      accent: "don’t say no.",
      body: "caveira drives abliterated open-weight models the way Claude Code drives a frontier one — it reads, edits and runs code in your project.",
      primaryLabel: "get access",
      primaryHref: "/signup",
      secondaryLabel: "see the plans",
      secondaryHref: "/pricing",
      terminalNote: "caveira",
      shader: "cycle",
      align: "left",
    },
  },
  marquee: {
    label: "Ticker",
    blurb: "Scrolling band of short phrases.",
    defaults: {
      items: ["no refusals", "runs on your metal", "bring your own model"],
    },
  },
  statement: {
    label: "Statement",
    blurb: "One big two-tone sentence.",
    defaults: {
      lead: "Every hosted agent stops to argue with you. caveira doesn’t.",
      tail: "It runs the model you chose, on the hardware you own.",
      band: false,
    },
  },
  features: {
    label: "Feature grid",
    blurb: "Cards in a 2, 3 or 4 column grid.",
    defaults: {
      eyebrow: "capabilities",
      title: "An agent, not an assistant.",
      lede: "",
      columns: "3",
      numbered: true,
      cards: [
        { title: "No refusals", body: "Built for abliterated weights." },
        { title: "Reads, edits, runs", body: "The full agent loop." },
        { title: "Your machine", body: "Local-first by design." },
      ],
      band: false,
    },
  },
  split: {
    label: "Split",
    blurb: "Copy on one side, terminal on the other.",
    defaults: {
      eyebrow: "the terminal client",
      title: "Pair a machine in one command.",
      body: "Launch caveira, approve the code in your browser, and the CLI holds a revocable session.",
      primaryLabel: "connect a terminal",
      primaryHref: "/cli",
      secondaryLabel: "",
      secondaryHref: "",
      media: "terminal",
      terminalTitle: "caveira — pairing",
      terminalLines: ["$ caveira", "", "- opening browser…", "! code  K7QP-3ZMX", "+ ✓ device approved"],
      reverse: false,
      band: true,
    },
  },
  chips: {
    label: "Chip row",
    blurb: "A wrapped row of tags or model names.",
    defaults: {
      eyebrow: "models",
      title: "Use the weights they warned you about.",
      lede: "",
      items: ["dolphin-mixtral", "██████-uncensored", "your local gguf"],
      note: "",
      band: false,
    },
  },
  steps: {
    label: "Steps",
    blurb: "Numbered how-it-works rows.",
    defaults: {
      eyebrow: "how it works",
      title: "Three moves to the first diff.",
      lede: "",
      items: [
        { title: "Get access", body: "Make an account and pick a plan." },
        { title: "Pair your terminal", body: "Approve the device code." },
        { title: "Point it at a model", body: "Hand it a task." },
      ],
      band: true,
    },
  },
  pricing: {
    label: "Pricing",
    blurb: "The three plan cards.",
    defaults: {
      eyebrow: "plans",
      title: "One agent, three sizes.",
      lede: "",
      live: true,
      cards: [],
      band: false,
    },
  },
  quotes: {
    label: "Quotes",
    blurb: "Testimonials in a grid.",
    defaults: {
      eyebrow: "word of mouth",
      title: "",
      items: [
        { quote: "It just does the work.", name: "someone", role: "somewhere" },
      ],
      band: false,
    },
  },
  faq: {
    label: "FAQ",
    blurb: "Expanding question list.",
    defaults: {
      eyebrow: "faq",
      title: "The questions you were going to ask.",
      items: [
        { q: "What does “abliterated” mean?", a: "A small edit that removes a model’s refusal behaviour." },
      ],
      band: false,
    },
  },
  cta: {
    label: "Closing CTA",
    blurb: "Centred headline and buttons.",
    defaults: {
      headline: "Stop asking permission",
      accent: "from your own tools.",
      body: "Set up an account, pair a terminal, put a model to work.",
      primaryLabel: "get access",
      primaryHref: "/signup",
      secondaryLabel: "see pricing",
      secondaryHref: "/pricing",
      glow: true,
    },
  },
};

export const BLOCK_ORDER: BlockType[] = [
  "hero",
  "marquee",
  "statement",
  "features",
  "split",
  "chips",
  "steps",
  "pricing",
  "quotes",
  "faq",
  "cta",
];

/* ------------------------------------------------------------------ */
/* prop helpers — the renderer never trusts the JSON's shape            */
/* ------------------------------------------------------------------ */

export function str(props: BlockProps, key: string, fallback = ""): string {
  const v = props[key];
  return typeof v === "string" ? v : fallback;
}

export function bool(props: BlockProps, key: string, fallback = false): boolean {
  const v = props[key];
  return typeof v === "boolean" ? v : fallback;
}

export function strings(props: BlockProps, key: string): string[] {
  const v = props[key];
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : [];
}

export function items(props: BlockProps, key: string): Record<string, string>[] {
  const v = props[key];
  if (!Array.isArray(v)) return [];
  return v.filter(
    (x): x is Record<string, string> => typeof x === "object" && x !== null && !Array.isArray(x),
  );
}

/** A short human label for a block, used in the editor's outline. */
export function blockSummary(block: Block): string {
  const p = block.props;
  const first =
    str(p, "title") ||
    str(p, "headline").split("\n")[0] ||
    str(p, "lead") ||
    strings(p, "items")[0] ||
    str(p, "eyebrow");
  return first || BLOCK_META[block.type].label;
}

export function newBlock(type: BlockType): Block {
  return {
    id:
      typeof crypto !== "undefined" && crypto.randomUUID
        ? crypto.randomUUID()
        : `${type}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
    type,
    // Deep copy so blocks never share an array with the palette defaults.
    props: JSON.parse(JSON.stringify(BLOCK_META[type].defaults)) as BlockProps,
  };
}
