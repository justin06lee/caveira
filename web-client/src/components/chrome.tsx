import Link from "next/link";
import type { ReactNode } from "react";

export function Wordmark({ className = "" }: { className?: string }) {
  return (
    <Link
      href="/"
      className={`group inline-flex items-center gap-2.5 font-mono text-sm tracking-[0.34em] text-bone uppercase ${className}`}
      aria-label="caveira home"
    >
      <span
        aria-hidden="true"
        className="grid h-5 w-5 place-items-center border border-ember/70 bg-ember/10 font-mono text-[11px] leading-none text-ember transition-colors group-hover:bg-ember/20"
      >
        c
      </span>
      caveira
    </Link>
  );
}

const NAV = [
  { href: "/#capabilities", label: "capabilities" },
  { href: "/#models", label: "models" },
  { href: "/pricing", label: "pricing" },
  { href: "/cli", label: "cli" },
];

export function SiteHeader({ right }: { right?: ReactNode }) {
  return (
    <header className="sticky top-0 z-50 border-b border-edge/80 bg-void/70 backdrop-blur-xl">
      <div className="mx-auto flex w-full max-w-6xl items-center justify-between gap-4 px-5 py-3.5 sm:px-8">
        <div className="flex items-center gap-8">
          <Wordmark />
          <nav className="hidden items-center gap-6 text-sm md:flex">
            {NAV.map((item) => (
              <Link key={item.href} href={item.href} className="nav-link">
                {item.label}
              </Link>
            ))}
          </nav>
        </div>
        <nav className="flex items-center gap-3 text-sm sm:gap-4">
          {right ?? (
            <>
              <Link href="/login" className="nav-link hidden sm:inline">
                log in
              </Link>
              <Link href="/signup" className="btn btn-primary px-4 py-2 text-sm">
                get access
              </Link>
            </>
          )}
        </nav>
      </div>
    </header>
  );
}

const FOOTER_COLUMNS: { title: string; links: { href: string; label: string }[] }[] = [
  {
    title: "product",
    links: [
      { href: "/#capabilities", label: "capabilities" },
      { href: "/#models", label: "models" },
      { href: "/pricing", label: "pricing" },
      { href: "/cli", label: "terminal client" },
    ],
  },
  {
    title: "account",
    links: [
      { href: "/signup", label: "sign up" },
      { href: "/login", label: "log in" },
      { href: "/account", label: "your account" },
    ],
  },
  {
    title: "the fine print",
    links: [
      { href: "/#capabilities", label: "what it does" },
      { href: "/#faq", label: "faq" },
      { href: "/pricing", label: "billing" },
    ],
  },
];

export function SiteFooter() {
  return (
    <footer className="mt-auto border-t border-edge bg-void-2">
      <div className="mx-auto w-full max-w-6xl px-5 py-12 sm:px-8">
        <div className="flex flex-col gap-10 md:flex-row md:justify-between">
          <div className="max-w-xs">
            <Wordmark />
            <p className="mt-4 text-sm leading-relaxed text-slate">
              A coding agent for abliterated models. Your model, your machine,
              no refusals.
            </p>
          </div>
          <div className="grid grid-cols-2 gap-8 sm:grid-cols-3 sm:gap-14">
            {FOOTER_COLUMNS.map((col) => (
              <div key={col.title}>
                <p className="eyebrow">{col.title}</p>
                <ul className="mt-4 flex flex-col gap-2.5">
                  {col.links.map((link) => (
                    <li key={link.label}>
                      <Link href={link.href} className="nav-link text-sm">
                        {link.label}
                      </Link>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        </div>
        <div className="mt-12 flex flex-col gap-2 border-t border-edge pt-6 text-xs text-slate-deep sm:flex-row sm:items-center sm:justify-between">
          <span className="font-mono tracking-[0.25em] uppercase">
            caveira © {new Date().getFullYear()}
          </span>
          <span className="font-mono">runs local. answers to no one.</span>
        </div>
      </div>
    </footer>
  );
}

/** Centered column used by the auth, cli and account pages. */
export function CenteredMain({
  children,
  width = "max-w-md",
}: {
  children: ReactNode;
  width?: string;
}) {
  return (
    <main className="relative flex flex-1 items-center justify-center px-5 py-12 sm:px-8 sm:py-20">
      <div
        aria-hidden="true"
        className="dot-backdrop pointer-events-none absolute inset-0 opacity-40 mask-fade-b"
      />
      <div className={`relative w-full ${width}`}>{children}</div>
    </main>
  );
}

export function Notice({
  tone = "error",
  children,
  role,
}: {
  tone?: "error" | "info" | "success";
  children: ReactNode;
  role?: "alert" | "status";
}) {
  const tones = {
    error: "border-ember/45 bg-ember/10 text-bone",
    info: "border-edge bg-edge-soft text-bone-dim",
    success: "border-brass/45 bg-brass/10 text-bone",
  } as const;
  return (
    <p
      role={role ?? (tone === "error" ? "alert" : "status")}
      className={`rounded-lg border px-3 py-2 text-sm ${tones[tone]}`}
    >
      {children}
    </p>
  );
}
