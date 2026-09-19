import Link from "next/link";
import type { ReactNode } from "react";

import { ShaderField, type ShaderVariant } from "@/components/effects/hero-shader";
import { FaqItem, FauxTerminal, Marquee, StepRow } from "@/components/marketing";
import { PLANS } from "@/lib/plans";
import {
  bool,
  items,
  str,
  strings,
  type Block,
  type BlockProps,
} from "@/lib/site-blocks";

/**
 * Draws a layout built in the site editor. Every block renders in the real
 * caveira design language, so the editor preview and the shipped page are the
 * same pixels — no server-only imports, so it works in both.
 */
export function BlockRenderer({ blocks }: { blocks: Block[] }) {
  return (
    <>
      {blocks.map((block) => (
        <RenderBlock key={block.id} block={block} />
      ))}
    </>
  );
}

export function RenderBlock({ block }: { block: Block }) {
  const p = block.props;
  switch (block.type) {
    case "hero":
      return <HeroBlock p={p} />;
    case "marquee":
      return <Marquee items={strings(p, "items")} />;
    case "statement":
      return <StatementBlock p={p} />;
    case "features":
      return <FeaturesBlock p={p} />;
    case "split":
      return <SplitBlock p={p} />;
    case "chips":
      return <ChipsBlock p={p} />;
    case "steps":
      return <StepsBlock p={p} />;
    case "pricing":
      return <PricingBlock p={p} />;
    case "quotes":
      return <QuotesBlock p={p} />;
    case "faq":
      return <FaqBlock p={p} />;
    case "cta":
      return <CtaBlock p={p} />;
    default:
      return null;
  }
}

/* ------------------------------------------------------------------ */
/* shared pieces                                                        */
/* ------------------------------------------------------------------ */

/** Wraps a section, optionally in a raised band with rules above and below. */
function Band({ on, children }: { on: boolean; children: ReactNode }) {
  if (!on) return <>{children}</>;
  return <div className="border-y border-edge bg-void-2">{children}</div>;
}

function Eyebrow({ children }: { children: string }) {
  if (!children) return null;
  return (
    <p className="eyebrow flex items-center gap-3">
      <span aria-hidden className="h-px w-8 bg-ember/70" />
      {children}
    </p>
  );
}

function Head({ p }: { p: BlockProps }) {
  const eyebrow = str(p, "eyebrow");
  const title = str(p, "title");
  const lede = str(p, "lede");
  if (!eyebrow && !title && !lede) return null;
  return (
    <div className="max-w-2xl">
      <Eyebrow>{eyebrow}</Eyebrow>
      {title && (
        <h2 className="mt-4 text-3xl font-semibold tracking-tight text-bone sm:text-4xl">
          {title}
        </h2>
      )}
      {lede && (
        <p className="mt-4 text-base leading-relaxed text-slate sm:text-lg">{lede}</p>
      )}
    </div>
  );
}

function Shell({ children }: { children: ReactNode }) {
  return (
    <section className="mx-auto w-full max-w-6xl px-5 py-20 sm:px-8 sm:py-28">
      {children}
    </section>
  );
}

/** Renders whichever of the two buttons actually have labels. */
function Buttons({
  p,
  size = "",
  center = false,
}: {
  p: BlockProps;
  size?: string;
  center?: boolean;
}) {
  const primary = str(p, "primaryLabel");
  const secondary = str(p, "secondaryLabel");
  if (!primary && !secondary) return null;
  return (
    <div
      className={`mt-9 flex flex-col gap-3 sm:flex-row sm:items-center ${
        center ? "justify-center" : ""
      }`}
    >
      {primary && (
        <Link href={str(p, "primaryHref", "#")} className={`btn btn-primary ${size}`}>
          {primary}
        </Link>
      )}
      {secondary && (
        <Link href={str(p, "secondaryHref", "#")} className={`btn btn-ghost ${size}`}>
          {secondary}
        </Link>
      )}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* blocks                                                               */
/* ------------------------------------------------------------------ */

function HeroBlock({ p }: { p: BlockProps }) {
  const centered = str(p, "align", "left") === "center";
  const shader = str(p, "shader", "cycle");
  const chip = str(p, "chip");
  const note = str(p, "terminalNote");

  return (
    <section className="relative overflow-hidden border-b border-edge">
      <div aria-hidden className="absolute inset-0 grid-backdrop opacity-[0.35]" />
      {shader !== "none" && (
        <div aria-hidden className="pointer-events-none absolute inset-0 opacity-70">
          <ShaderField variant={shader as ShaderVariant} />
        </div>
      )}
      <div
        aria-hidden
        className="absolute inset-0 bg-gradient-to-b from-void/70 via-void/85 to-void"
      />
      <div
        aria-hidden
        className="absolute inset-0 bg-[radial-gradient(ellipse_at_top,transparent_10%,var(--color-void)_78%)]"
      />

      <div className="relative mx-auto w-full max-w-6xl px-5 pb-20 pt-20 sm:px-8 sm:pb-28 sm:pt-28">
        <div className={centered ? "mx-auto max-w-3xl text-center" : "max-w-3xl"}>
          {chip && (
            <span className="chip chip-ember">
              <span className="h-1.5 w-1.5 rounded-full bg-ember-bright" />
              {chip}
            </span>
          )}

          <h1 className="mt-6 text-[2.75rem] font-semibold leading-[1.02] tracking-tight text-bone sm:text-6xl lg:text-7xl">
            {str(p, "headline")
              .split("\n")
              .map((line, i) => (
                <span key={i} className="block">
                  {line}
                </span>
              ))}
            {str(p, "accent") && (
              <span className="block text-ember">{str(p, "accent")}</span>
            )}
          </h1>

          {str(p, "body") && (
            <p
              className={`mt-7 max-w-xl text-lg leading-relaxed text-bone-dim ${
                centered ? "mx-auto" : ""
              }`}
            >
              {str(p, "body")}
            </p>
          )}

          <Buttons p={p} size="px-6 py-3.5 text-base" center={centered} />

          {note && (
            <div className="mt-8 inline-flex items-center gap-3 rounded-lg border border-edge bg-[#08080a]/80 px-4 py-2.5 font-mono text-sm text-bone-dim backdrop-blur">
              <span className="text-ember">$</span>
              <span>{note}</span>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}

function StatementBlock({ p }: { p: BlockProps }) {
  return (
    <Band on={bool(p, "band")}>
      <Shell>
        <p className="max-w-4xl text-2xl font-medium leading-snug tracking-tight text-bone sm:text-3xl">
          {str(p, "lead")}{" "}
          <span className="text-slate">{str(p, "tail")}</span>
        </p>
      </Shell>
    </Band>
  );
}

function FeaturesBlock({ p }: { p: BlockProps }) {
  const cards = items(p, "cards");
  const cols = str(p, "columns", "3");
  const numbered = bool(p, "numbered", true);
  const grid =
    cols === "2"
      ? "md:grid-cols-2"
      : cols === "4"
        ? "md:grid-cols-2 lg:grid-cols-4"
        : "md:grid-cols-2 lg:grid-cols-3";

  return (
    <Band on={bool(p, "band")}>
      <Shell>
        <Head p={p} />
        <div className={`mt-12 grid grid-cols-1 gap-4 ${grid}`}>
          {cards.map((card, i) => (
            <div
              key={i}
              className="group panel relative flex flex-col overflow-hidden p-6 transition-colors hover:border-edge-bright"
            >
              {numbered && (
                <span
                  aria-hidden
                  className="pointer-events-none absolute -right-6 -top-8 font-mono text-[5.5rem] leading-none text-edge-soft transition-colors group-hover:text-edge"
                >
                  {String(i + 1).padStart(2, "0")}
                </span>
              )}
              <h3 className="relative text-lg font-semibold text-bone">{card.title}</h3>
              <p className="relative mt-3 text-sm leading-relaxed text-slate">
                {card.body}
              </p>
            </div>
          ))}
        </div>
      </Shell>
    </Band>
  );
}

/** "+ " green, "! " red, "- " grey, anything else bone. */
function parseTerminalLine(raw: string): {
  text: string;
  tone: "acid" | "ember" | "dim" | "bone";
} {
  if (raw.startsWith("+ ")) return { text: raw.slice(2), tone: "acid" };
  if (raw.startsWith("! ")) return { text: raw.slice(2), tone: "ember" };
  if (raw.startsWith("- ")) return { text: raw.slice(2), tone: "dim" };
  return { text: raw, tone: "bone" };
}

function SplitBlock({ p }: { p: BlockProps }) {
  const media = str(p, "media", "terminal");
  const reverse = bool(p, "reverse");

  const copy = (
    <div>
      <Eyebrow>{str(p, "eyebrow")}</Eyebrow>
      {str(p, "title") && (
        <h2 className="mt-4 text-3xl font-semibold tracking-tight text-bone sm:text-4xl">
          {str(p, "title")}
        </h2>
      )}
      {str(p, "body") && (
        <p className="mt-4 text-base leading-relaxed text-slate sm:text-lg">
          {str(p, "body")}
        </p>
      )}
      <Buttons p={p} />
    </div>
  );

  const visual =
    media === "terminal" ? (
      <FauxTerminal
        title={str(p, "terminalTitle", "caveira")}
        lines={strings(p, "terminalLines").map(parseTerminalLine)}
      />
    ) : media === "shader" ? (
      <div className="h-72 overflow-hidden rounded-xl border border-edge bg-[#08080a] p-3">
        <ShaderField variant="tunnel" className="h-full w-full text-ember/50" size={11} />
      </div>
    ) : null;

  return (
    <Band on={bool(p, "band")}>
      <section className="mx-auto w-full max-w-6xl px-5 py-20 sm:px-8 sm:py-24">
        <div className="grid items-center gap-12 lg:grid-cols-2">
          {reverse ? (
            <>
              <div className="order-2 lg:order-1">{visual}</div>
              <div className="order-1 lg:order-2">{copy}</div>
            </>
          ) : (
            <>
              {copy}
              {visual}
            </>
          )}
        </div>
      </section>
    </Band>
  );
}

function ChipsBlock({ p }: { p: BlockProps }) {
  return (
    <Band on={bool(p, "band")}>
      <Shell>
        <Head p={p} />
        <div className="mt-10 flex flex-wrap gap-3">
          {strings(p, "items").map((item) => (
            <span
              key={item}
              className="inline-flex items-center gap-2 rounded-md border border-edge bg-panel px-3.5 py-2 font-mono text-sm text-bone-dim"
            >
              <span className="h-1.5 w-1.5 rounded-full bg-ember/70" />
              {item}
            </span>
          ))}
        </div>
        {str(p, "note") && (
          <p className="mt-6 font-mono text-xs text-slate-deep">{str(p, "note")}</p>
        )}
      </Shell>
    </Band>
  );
}

function StepsBlock({ p }: { p: BlockProps }) {
  const steps = items(p, "items");
  return (
    <Band on={bool(p, "band")}>
      <section className="mx-auto w-full max-w-6xl px-5 py-20 sm:px-8 sm:py-24">
        <div className="grid gap-12 lg:grid-cols-[0.9fr_1.1fr]">
          <Head p={p} />
          <div>
            {steps.map((step, i) => (
              <StepRow key={i} n={i + 1} title={step.title ?? ""}>
                {step.body}
              </StepRow>
            ))}
          </div>
        </div>
      </section>
    </Band>
  );
}

function PricingBlock({ p }: { p: BlockProps }) {
  const live = bool(p, "live", true);
  const cards = live
    ? PLANS.map((plan) => ({
        name: plan.name,
        price: `$${plan.priceUsd}`,
        tagline: plan.tagline,
        features: plan.features.join("\n"),
      }))
    : items(p, "cards").map((c) => ({
        name: c.name ?? "",
        price: c.price ?? "",
        tagline: c.tagline ?? "",
        features: c.features ?? "",
      }));

  return (
    <Band on={bool(p, "band")}>
      <Shell>
        <Head p={p} />
        <div className="mt-12 grid grid-cols-1 gap-5 md:grid-cols-3">
          {cards.map((card, i) => {
            const featured = i === 1;
            return (
              <div
                key={i}
                className={`panel flex flex-col p-6 ${featured ? "border-ember/60 md:-mt-3 md:pb-9" : ""}`}
              >
                <h3 className="text-lg font-semibold text-bone">{card.name}</h3>
                <p className="mt-2 text-sm text-slate">{card.tagline}</p>
                <p className="mt-5 font-mono text-3xl text-bone">{card.price}</p>
                <ul className="mt-5 flex flex-col gap-2 text-sm text-bone-dim">
                  {card.features
                    .split("\n")
                    .filter(Boolean)
                    .map((f) => (
                      <li key={f} className="flex gap-2">
                        <span
                          aria-hidden
                          className="mt-2 h-1 w-1 shrink-0 rounded-[1px] bg-slate-deep"
                        />
                        {f}
                      </li>
                    ))}
                </ul>
                <div className="mt-auto pt-7">
                  <Link
                    href="/pricing"
                    className={`btn w-full ${featured ? "btn-primary" : "btn-ghost"}`}
                  >
                    {card.name ? `get ${card.name.toLowerCase()}` : "choose"}
                  </Link>
                </div>
              </div>
            );
          })}
        </div>
      </Shell>
    </Band>
  );
}

function QuotesBlock({ p }: { p: BlockProps }) {
  const quotes = items(p, "items");
  return (
    <Band on={bool(p, "band")}>
      <Shell>
        <Head p={p} />
        <div className="mt-12 grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
          {quotes.map((q, i) => (
            <figure key={i} className="panel flex flex-col p-6">
              <blockquote className="text-base leading-relaxed text-bone">
                “{q.quote}”
              </blockquote>
              <figcaption className="mt-5 font-mono text-xs tracking-[0.14em] text-slate uppercase">
                {q.name}
                {q.role ? ` · ${q.role}` : ""}
              </figcaption>
            </figure>
          ))}
        </div>
      </Shell>
    </Band>
  );
}

function FaqBlock({ p }: { p: BlockProps }) {
  return (
    <Band on={bool(p, "band")}>
      <Shell>
        <Head p={p} />
        <div className="mt-10">
          {items(p, "items").map((item, i) => (
            <FaqItem key={i} q={item.q ?? ""}>
              {item.a}
            </FaqItem>
          ))}
        </div>
      </Shell>
    </Band>
  );
}

function CtaBlock({ p }: { p: BlockProps }) {
  return (
    <section className="relative overflow-hidden border-t border-edge">
      <div aria-hidden className="absolute inset-0 grid-backdrop opacity-30" />
      {bool(p, "glow", true) && (
        <div
          aria-hidden
          className="absolute inset-0 bg-[radial-gradient(ellipse_at_center,color-mix(in_srgb,var(--color-ember)_16%,transparent),transparent_60%)]"
        />
      )}
      <div className="relative mx-auto w-full max-w-6xl px-5 py-24 text-center sm:px-8 sm:py-32">
        <h2 className="mx-auto max-w-2xl text-4xl font-semibold tracking-tight text-bone sm:text-5xl">
          {str(p, "headline")}
          {str(p, "accent") && (
            <span className="block text-ember">{str(p, "accent")}</span>
          )}
        </h2>
        {str(p, "body") && (
          <p className="mx-auto mt-5 max-w-lg text-base text-slate sm:text-lg">
            {str(p, "body")}
          </p>
        )}
        <Buttons p={p} size="px-7 py-3.5 text-base" center />
      </div>
    </section>
  );
}
