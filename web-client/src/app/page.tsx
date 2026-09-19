import Link from "next/link";

import { SiteFooter, SiteHeader } from "@/components/chrome";
import { HeroShader } from "@/components/effects/hero-shader";
import {
  FaqItem,
  FauxTerminal,
  FeatureCard,
  Marquee,
  Section,
  StepRow,
} from "@/components/marketing";

const MARQUEE = [
  "no refusals",
  "abliterated weights",
  "runs on your metal",
  "reads · edits · executes",
  "terminal + web",
  "answers to no one",
  "bring your own model",
];

const MODELS = [
  "dolphin-mixtral",
  "wizard-vicuna",
  "██████-uncensored",
  "nous-hermes",
  "llama-3-████████",
  "qwen-abliterated",
  "your local gguf",
];

export default function Home() {
  return (
    <>
      <SiteHeader />
      <main className="flex flex-1 flex-col">
        {/* ---- hero -------------------------------------------------- */}
        <section className="relative overflow-hidden border-b border-edge">
          <div aria-hidden className="absolute inset-0 grid-backdrop opacity-[0.35]" />
          <div aria-hidden className="pointer-events-none absolute inset-0 opacity-70">
            <HeroShader />
          </div>
          {/* Scrim keeps the headline legible over the moving field. */}
          <div
            aria-hidden
            className="absolute inset-0 bg-gradient-to-b from-void/70 via-void/85 to-void"
          />
          <div
            aria-hidden
            className="absolute inset-0 bg-[radial-gradient(ellipse_at_top,transparent_10%,var(--color-void)_78%)]"
          />

          <div className="relative mx-auto w-full max-w-6xl px-5 pb-20 pt-20 sm:px-8 sm:pb-28 sm:pt-28">
            <div className="max-w-3xl">
              <span className="chip chip-ember">
                <span className="h-1.5 w-1.5 rounded-full bg-ember-bright" />
                abliterated · uncensored · local
              </span>

              <h1 className="mt-6 text-[2.75rem] font-semibold leading-[1.02] tracking-tight text-bone sm:text-6xl lg:text-7xl">
                the coding agent
                <br />
                for models that
                <br />
                <span className="text-ember">don&rsquo;t say no.</span>
              </h1>

              <p className="mt-7 max-w-xl text-lg leading-relaxed text-bone-dim">
                caveira drives abliterated open-weight models the way Claude
                Code drives a frontier one — it reads, edits and runs code in
                your project. No guardrails, no lectures, no telemetry home.
                Your model, your machine.
              </p>

              <div className="mt-9 flex flex-col gap-3 sm:flex-row sm:items-center">
                <Link href="/signup" className="btn btn-primary px-6 py-3.5 text-base">
                  get access
                </Link>
                <Link href="/pricing" className="btn btn-ghost px-6 py-3.5 text-base">
                  see the plans
                </Link>
              </div>

              <div className="mt-8 inline-flex items-center gap-3 rounded-lg border border-edge bg-[#08080a]/80 px-4 py-2.5 font-mono text-sm text-bone-dim backdrop-blur">
                <span className="text-ember">$</span>
                <span>caveira</span>
                <span className="text-slate-deep">
                  # pair a terminal from{" "}
                  <Link href="/cli" className="link">
                    /cli
                  </Link>
                </span>
              </div>
            </div>
          </div>
        </section>

        <Marquee items={MARQUEE} />

        {/* ---- manifesto -------------------------------------------- */}
        <Section className="!pb-10">
          <p className="max-w-4xl text-2xl font-medium leading-snug tracking-tight text-bone sm:text-3xl">
            Every hosted agent stops to argue with you. caveira doesn&rsquo;t.{" "}
            <span className="text-slate">
              It runs the model you chose, on the hardware you own, and does the
              work you actually asked for.
            </span>
          </p>
        </Section>

        {/* ---- capabilities ----------------------------------------- */}
        <Section
          id="capabilities"
          eyebrow="capabilities"
          title="An agent, not an assistant."
          lede="It lives in your repo and your terminal, does the loop end to end, and never phones a policy server before touching a file."
        >
          <div className="mt-12 grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
            <FeatureCard index="01" title="No refusals">
              Built for abliterated weights — models with the refusal reflex
              removed. Security research, red-teaming, the ugly legacy code
              nobody will touch: it just does it.
            </FeatureCard>
            <FeatureCard index="02" title="Reads, edits, runs">
              The full agent loop. It opens files, writes diffs, runs commands
              and reads the output back — iterating until the task is done, not
              until it&rsquo;s uncomfortable.
            </FeatureCard>
            <FeatureCard index="03" title="Your machine">
              Local-first by design. Point it at a model on your own GPU and
              nothing leaves the box. No prompts logged, no code shipped to a
              vendor.
            </FeatureCard>
            <FeatureCard index="04" title="Bring your own weights">
              Any GGUF, any endpoint. Dolphin, Hermes, an uncensored fine-tune
              you cooked yourself — if it speaks tokens, caveira drives it.
            </FeatureCard>
            <FeatureCard index="05" title="Terminal + web">
              A Bubble Tea TUI for the people who live in a shell, and a web
              client for everything else. One account, one session, both
              clients.
            </FeatureCard>
            <FeatureCard index="06" title="Answers to no one">
              No content filter sitting between you and your own compute. The
              only limits are the ones your prompt sets.
            </FeatureCard>
          </div>
        </Section>

        {/* ---- terminal showcase ------------------------------------ */}
        <div className="border-y border-edge bg-void-2">
          <Section className="!py-20 sm:!py-24">
            <div className="grid items-center gap-12 lg:grid-cols-2">
              <div>
                <p className="eyebrow flex items-center gap-3">
                  <span aria-hidden className="h-px w-8 bg-ember/70" />
                  the terminal client
                </p>
                <h2 className="mt-4 text-3xl font-semibold tracking-tight text-bone sm:text-4xl">
                  Pair a machine in one command.
                </h2>
                <p className="mt-4 text-base leading-relaxed text-slate sm:text-lg">
                  Launch <code className="font-mono text-bone-dim">caveira</code>,
                  it shows a short code and opens your browser. Approve the code
                  and the CLI holds a revocable session in{" "}
                  <code className="font-mono text-bone-dim">~/.caveira</code>. No
                  password ever touches the terminal.
                </p>
                <div className="mt-8 flex flex-col gap-3 sm:flex-row">
                  <Link href="/cli" className="btn btn-ghost">
                    connect a terminal
                  </Link>
                  <Link href="/signup" className="btn btn-quiet text-sm">
                    create an account first
                  </Link>
                </div>
              </div>
              <FauxTerminal
                title="caveira — pairing"
                lines={[
                  { text: "$ caveira", tone: "bone" },
                  { text: "", tone: "dim" },
                  { text: "  no session found. opening browser…", tone: "dim" },
                  { text: "  visit  caveira.dev/cli", tone: "dim" },
                  { text: "  code   K7QP-3ZMX", tone: "ember" },
                  { text: "", tone: "dim" },
                  { text: "  ✓ device approved — signed in", tone: "acid" },
                  { text: "  ✓ model  dolphin-mixtral @ localhost:11434", tone: "acid" },
                  { text: "", tone: "dim" },
                  { text: "› refactor the auth layer and run the tests", tone: "bone" },
                  { text: "  reading  src/lib/auth.ts …", tone: "dim" },
                  { text: "  editing  3 files · +214 -96", tone: "dim" },
                  { text: "  running  bun test  →  42 passed", tone: "acid" },
                ]}
              />
            </div>
          </Section>
        </div>

        {/* ---- models ----------------------------------------------- */}
        <Section
          id="models"
          eyebrow="models"
          title="Use the weights they warned you about."
          lede="caveira doesn't ship a model — it drives yours. Any abliterated or uncensored open-weight model, local or self-hosted, through one adapter."
        >
          <div className="mt-10 flex flex-wrap gap-3">
            {MODELS.map((m) => (
              <span
                key={m}
                className="inline-flex items-center gap-2 rounded-md border border-edge bg-panel px-3.5 py-2 font-mono text-sm text-bone-dim"
              >
                <span className="h-1.5 w-1.5 rounded-full bg-ember/70" />
                {m}
              </span>
            ))}
          </div>
          <p className="mt-6 font-mono text-xs text-slate-deep">
            // redacted entries are yours to fill in. we don&rsquo;t keep a
            list of what you&rsquo;re allowed to run.
          </p>
        </Section>

        {/* ---- how it works ----------------------------------------- */}
        <div className="border-y border-edge bg-void-2">
          <Section className="!py-20 sm:!py-24">
            <div className="grid gap-12 lg:grid-cols-[0.9fr_1.1fr]">
              <div>
                <p className="eyebrow flex items-center gap-3">
                  <span aria-hidden className="h-px w-8 bg-ember/70" />
                  how it works
                </p>
                <h2 className="mt-4 text-3xl font-semibold tracking-tight text-bone sm:text-4xl">
                  From download to first diff in three moves.
                </h2>
                <p className="mt-4 text-base leading-relaxed text-slate">
                  The whole flow works on a fresh checkout with nothing
                  configured. Add your model when you&rsquo;re ready.
                </p>
              </div>
              <div>
                <StepRow n={1} title="Get access">
                  Make an account and pick a plan. Billing runs in dev mode out
                  of the box, so you can try the entire loop without a card.
                </StepRow>
                <StepRow n={2} title="Pair your terminal">
                  Run the CLI, approve the device code in your browser, and the
                  machine is signed in — revocable any time from your account.
                </StepRow>
                <StepRow n={3} title="Point it at a model">
                  Aim caveira at a local or self-hosted abliterated model and
                  hand it a task. It reads, edits and runs until it&rsquo;s done.
                </StepRow>
              </div>
            </div>
          </Section>
        </div>

        {/* ---- faq -------------------------------------------------- */}
        <Section id="faq" eyebrow="faq" title="The questions you were going to ask.">
          <div className="mt-10">
            <FaqItem q="What does “abliterated” mean?">
              An abliteration is a small edit to an open-weight model that
              removes its refusal behaviour without retraining it. The model
              keeps its capabilities and drops the reflex to say no. caveira is
              built to drive exactly these models.
            </FaqItem>
            <FaqItem q="Is any of this hosted by you?">
              No. The backend handles accounts and billing only. The model runs
              wherever you point it — a local runtime on your own hardware, or
              your own endpoint. Your code and prompts don&rsquo;t pass through us.
            </FaqItem>
            <FaqItem q="Is this legal / is this safe?">
              caveira is a tool for people doing legitimate work — security
              research, red-teaming, and the code hosted agents refuse to touch.
              What you run on your own machine is your responsibility, same as
              any compiler or shell.
            </FaqItem>
            <FaqItem q="Do I need a subscription to try it?">
              Not to look around. Until Stripe is configured the backend runs in
              dev billing mode, where choosing a plan activates it instantly with
              no payment, so the full flow works locally.
            </FaqItem>
          </div>
        </Section>

        {/* ---- final CTA -------------------------------------------- */}
        <section className="relative overflow-hidden border-t border-edge">
          <div aria-hidden className="absolute inset-0 grid-backdrop opacity-30" />
          <div
            aria-hidden
            className="absolute inset-0 bg-[radial-gradient(ellipse_at_center,color-mix(in_srgb,var(--color-ember)_16%,transparent),transparent_60%)]"
          />
          <div className="relative mx-auto w-full max-w-6xl px-5 py-24 text-center sm:px-8 sm:py-32">
            <h2 className="mx-auto max-w-2xl text-4xl font-semibold tracking-tight text-bone sm:text-5xl">
              Stop asking permission
              <br />
              <span className="text-ember">from your own tools.</span>
            </h2>
            <p className="mx-auto mt-5 max-w-lg text-base text-slate sm:text-lg">
              Set up an account, pair a terminal, and put an uncensored model to
              work in your codebase.
            </p>
            <div className="mt-9 flex flex-col justify-center gap-3 sm:flex-row">
              <Link href="/signup" className="btn btn-primary px-7 py-3.5 text-base">
                get access
              </Link>
              <Link href="/pricing" className="btn btn-ghost px-7 py-3.5 text-base">
                see pricing
              </Link>
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </>
  );
}
