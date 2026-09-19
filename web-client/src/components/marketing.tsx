import type { ReactNode } from "react";

/** Section shell: an id anchor, a mono eyebrow, a heading and optional lede. */
export function Section({
  id,
  eyebrow,
  title,
  lede,
  children,
  className = "",
}: {
  id?: string;
  eyebrow?: string;
  title?: ReactNode;
  lede?: ReactNode;
  children?: ReactNode;
  className?: string;
}) {
  return (
    <section
      id={id}
      className={`mx-auto w-full max-w-6xl px-5 py-20 sm:px-8 sm:py-28 ${className}`}
    >
      {(eyebrow || title || lede) && (
        <div className="max-w-2xl">
          {eyebrow && (
            <p className="eyebrow flex items-center gap-3">
              <span aria-hidden className="h-px w-8 bg-ember/70" />
              {eyebrow}
            </p>
          )}
          {title && (
            <h2 className="mt-4 text-3xl font-semibold tracking-tight text-bone sm:text-4xl">
              {title}
            </h2>
          )}
          {lede && (
            <p className="mt-4 text-base leading-relaxed text-slate sm:text-lg">
              {lede}
            </p>
          )}
        </div>
      )}
      {children}
    </section>
  );
}

/** A single scrolling ticker band — the "wanted poster" strip. */
export function Marquee({ items }: { items: string[] }) {
  const row = [...items, ...items];
  return (
    <div className="relative border-y border-edge bg-void-2 py-3.5">
      <div className="mask-fade-edges overflow-hidden">
        <div className="animate-marquee flex w-max items-center gap-10 whitespace-nowrap">
          {row.map((item, i) => (
            <span
              key={i}
              className="flex items-center gap-10 font-mono text-xs tracking-[0.22em] text-bone-dim uppercase"
            >
              {item}
              <span aria-hidden className="text-ember">
                //
              </span>
            </span>
          ))}
        </div>
      </div>
    </div>
  );
}

export function FeatureCard({
  index,
  title,
  children,
}: {
  index: string;
  title: string;
  children: ReactNode;
}) {
  return (
    <div className="group relative flex flex-col overflow-hidden panel p-6 transition-colors hover:border-edge-bright">
      <span
        aria-hidden
        className="pointer-events-none absolute -right-6 -top-8 font-mono text-[5.5rem] leading-none text-edge-soft transition-colors group-hover:text-edge"
      >
        {index}
      </span>
      <h3 className="relative text-lg font-semibold text-bone">{title}</h3>
      <p className="relative mt-3 text-sm leading-relaxed text-slate">
        {children}
      </p>
    </div>
  );
}

/** A fake terminal window used to show the pairing flow. */
export function FauxTerminal({
  title = "caveira",
  lines,
}: {
  title?: string;
  lines: { text: string; tone?: "dim" | "ember" | "bone" | "acid" }[];
}) {
  const toneClass: Record<string, string> = {
    dim: "text-slate",
    ember: "text-ember-bright",
    bone: "text-bone",
    acid: "text-acid",
  };
  return (
    <div className="scanlines relative overflow-hidden rounded-xl border border-edge bg-[#08080a] shadow-[0_30px_80px_-40px_rgba(229,67,58,0.35)]">
      <div className="flex items-center gap-2 border-b border-edge bg-void-2 px-4 py-2.5">
        <span className="h-2.5 w-2.5 rounded-full bg-edge-bright" />
        <span className="h-2.5 w-2.5 rounded-full bg-edge-bright" />
        <span className="h-2.5 w-2.5 rounded-full bg-ember/70" />
        <span className="ml-2 font-mono text-xs tracking-[0.2em] text-slate uppercase">
          {title}
        </span>
      </div>
      <pre className="overflow-x-auto p-5 font-mono text-[13px] leading-relaxed">
        {lines.map((line, i) => (
          <div key={i} className={toneClass[line.tone ?? "bone"]}>
            {line.text || " "}
          </div>
        ))}
      </pre>
    </div>
  );
}

export function StepRow({
  n,
  title,
  children,
}: {
  n: number;
  title: string;
  children: ReactNode;
}) {
  return (
    <div className="flex gap-5 border-t border-edge py-7 first:border-t-0 first:pt-0">
      <span className="mt-0.5 grid h-8 w-8 shrink-0 place-items-center border border-ember/50 font-mono text-sm text-ember">
        {n}
      </span>
      <div>
        <h3 className="text-base font-semibold text-bone">{title}</h3>
        <p className="mt-1.5 text-sm leading-relaxed text-slate">{children}</p>
      </div>
    </div>
  );
}

export function FaqItem({
  q,
  children,
}: {
  q: string;
  children: ReactNode;
}) {
  return (
    <details className="group border-t border-edge py-5 first:border-t-0">
      <summary className="flex cursor-pointer list-none items-center justify-between gap-4 text-base font-medium text-bone marker:hidden">
        {q}
        <span
          aria-hidden
          className="font-mono text-ember transition-transform group-open:rotate-45"
        >
          +
        </span>
      </summary>
      <p className="mt-3 max-w-2xl text-sm leading-relaxed text-slate">
        {children}
      </p>
    </details>
  );
}
