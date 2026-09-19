import type { SiteLayout } from "@/lib/site-blocks";

/**
 * The landing page as it ships today, expressed in blocks. The editor starts
 * here, so the first thing you see is the real site — rearrange from something
 * rather than from nothing.
 */
export const DEFAULT_LAYOUT: SiteLayout = {
  version: 1,
  blocks: [
    {
      id: "hero",
      type: "hero",
      props: {
        chip: "abliterated · uncensored · local",
        headline: "the coding agent\nfor models that",
        accent: "don’t say no.",
        body: "caveira drives abliterated open-weight models the way Claude Code drives a frontier one — it reads, edits and runs code in your project. No guardrails, no lectures, no telemetry home. Your model, your machine.",
        primaryLabel: "get access",
        primaryHref: "/signup",
        secondaryLabel: "see the plans",
        secondaryHref: "/pricing",
        terminalNote: "caveira",
        shader: "cycle",
        align: "left",
      },
    },
    {
      id: "ticker",
      type: "marquee",
      props: {
        items: [
          "no refusals",
          "abliterated weights",
          "runs on your metal",
          "reads · edits · executes",
          "terminal + web",
          "answers to no one",
          "bring your own model",
        ],
      },
    },
    {
      id: "manifesto",
      type: "statement",
      props: {
        lead: "Every hosted agent stops to argue with you. caveira doesn’t.",
        tail: "It runs the model you chose, on the hardware you own, and does the work you actually asked for.",
        band: false,
      },
    },
    {
      id: "capabilities",
      type: "features",
      props: {
        eyebrow: "capabilities",
        title: "An agent, not an assistant.",
        lede: "It lives in your repo and your terminal, does the loop end to end, and never phones a policy server before touching a file.",
        columns: "3",
        numbered: true,
        band: false,
        cards: [
          {
            title: "No refusals",
            body: "Built for abliterated weights — models with the refusal reflex removed. Security research, red-teaming, the ugly legacy code nobody will touch: it just does it.",
          },
          {
            title: "Reads, edits, runs",
            body: "The full agent loop. It opens files, writes diffs, runs commands and reads the output back — iterating until the task is done, not until it’s uncomfortable.",
          },
          {
            title: "Your machine",
            body: "Local-first by design. Point it at a model on your own GPU and nothing leaves the box. No prompts logged, no code shipped to a vendor.",
          },
          {
            title: "Bring your own weights",
            body: "Any GGUF, any endpoint. Dolphin, Hermes, an uncensored fine-tune you cooked yourself — if it speaks tokens, caveira drives it.",
          },
          {
            title: "Terminal + web",
            body: "A Bubble Tea TUI for the people who live in a shell, and a web client for everything else. One account, one session, both clients.",
          },
          {
            title: "Answers to no one",
            body: "No content filter sitting between you and your own compute. The only limits are the ones your prompt sets.",
          },
        ],
      },
    },
    {
      id: "terminal",
      type: "split",
      props: {
        eyebrow: "the terminal client",
        title: "Pair a machine in one command.",
        body: "Launch caveira, it shows a short code and opens your browser. Approve the code and the CLI holds a revocable session in ~/.caveira. No password ever touches the terminal.",
        primaryLabel: "connect a terminal",
        primaryHref: "/cli",
        secondaryLabel: "",
        secondaryHref: "",
        media: "terminal",
        terminalTitle: "caveira — pairing",
        terminalLines: [
          "$ caveira",
          "",
          "-   no session found. opening browser…",
          "-   visit  caveira.dev/cli",
          "!   code   K7QP-3ZMX",
          "",
          "+   ✓ device approved — signed in",
          "+   ✓ model  dolphin-mixtral @ localhost:11434",
          "",
          "› refactor the auth layer and run the tests",
          "-   reading  src/lib/auth.ts …",
          "-   editing  3 files · +214 -96",
          "+   running  bun test  →  42 passed",
        ],
        reverse: false,
        band: true,
      },
    },
    {
      id: "models",
      type: "chips",
      props: {
        eyebrow: "models",
        title: "Use the weights they warned you about.",
        lede: "caveira doesn't ship a model — it drives yours. Any abliterated or uncensored open-weight model, local or self-hosted, through one adapter.",
        items: [
          "dolphin-mixtral",
          "wizard-vicuna",
          "██████-uncensored",
          "nous-hermes",
          "llama-3-████████",
          "qwen-abliterated",
          "your local gguf",
        ],
        note: "// redacted entries are yours to fill in. we don’t keep a list of what you’re allowed to run.",
        band: false,
      },
    },
    {
      id: "how",
      type: "steps",
      props: {
        eyebrow: "how it works",
        title: "From download to first diff in three moves.",
        lede: "The whole flow works on a fresh checkout with nothing configured. Add your model when you’re ready.",
        band: true,
        items: [
          {
            title: "Get access",
            body: "Make an account and pick a plan. Billing runs in dev mode out of the box, so you can try the entire loop without a card.",
          },
          {
            title: "Pair your terminal",
            body: "Run the CLI, approve the device code in your browser, and the machine is signed in — revocable any time from your account.",
          },
          {
            title: "Point it at a model",
            body: "Aim caveira at a local or self-hosted abliterated model and hand it a task. It reads, edits and runs until it’s done.",
          },
        ],
      },
    },
    {
      id: "faq",
      type: "faq",
      props: {
        eyebrow: "faq",
        title: "The questions you were going to ask.",
        band: false,
        items: [
          {
            q: "What does “abliterated” mean?",
            a: "An abliteration is a small edit to an open-weight model that removes its refusal behaviour without retraining it. The model keeps its capabilities and drops the reflex to say no. caveira is built to drive exactly these models.",
          },
          {
            q: "Is any of this hosted by you?",
            a: "No. The backend handles accounts and billing only. The model runs wherever you point it — a local runtime on your own hardware, or your own endpoint. Your code and prompts don’t pass through us.",
          },
          {
            q: "Is this legal / is this safe?",
            a: "caveira is a tool for people doing legitimate work — security research, red-teaming, and the code hosted agents refuse to touch. What you run on your own machine is your responsibility, same as any compiler or shell.",
          },
          {
            q: "Do I need a subscription to try it?",
            a: "Not to look around. Until Stripe is configured the backend runs in dev billing mode, where choosing a plan activates it instantly with no payment, so the full flow works locally.",
          },
        ],
      },
    },
    {
      id: "closing",
      type: "cta",
      props: {
        headline: "Stop asking permission",
        accent: "from your own tools.",
        body: "Set up an account, pair a terminal, and put an uncensored model to work in your codebase.",
        primaryLabel: "get access",
        primaryHref: "/signup",
        secondaryLabel: "see pricing",
        secondaryHref: "/pricing",
        glow: true,
      },
    },
  ],
};
