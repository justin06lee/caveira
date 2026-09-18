import Link from "next/link";
import type { ReactNode } from "react";

export function Wordmark({ className = "" }: { className?: string }) {
  return (
    <Link
      href="/"
      className={`inline-flex items-center gap-2 font-mono text-sm tracking-[0.35em] text-bone uppercase ${className}`}
      aria-label="caveira home"
    >
      <span aria-hidden="true" className="h-2 w-2 rounded-[1px] bg-ember" />
      caveira
    </Link>
  );
}

export function SiteHeader({ right }: { right?: ReactNode }) {
  return (
    <header className="border-b border-edge">
      <div className="mx-auto flex w-full max-w-5xl items-center justify-between gap-4 px-5 py-4 sm:px-8">
        <Wordmark />
        <nav className="flex items-center gap-4 text-sm sm:gap-6">
          {right ?? (
            <>
              <Link href="/pricing" className="text-slate transition-colors hover:text-bone">
                Pricing
              </Link>
              <Link href="/login" className="text-slate transition-colors hover:text-bone">
                Log in
              </Link>
            </>
          )}
        </nav>
      </div>
    </header>
  );
}

export function SiteFooter() {
  return (
    <footer className="mt-auto border-t border-edge">
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-2 px-5 py-6 text-xs text-slate sm:flex-row sm:items-center sm:justify-between sm:px-8">
        <span className="font-mono tracking-[0.25em] uppercase">caveira</span>
        <span>Your model, your machine.</span>
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
    <main className="flex flex-1 items-center justify-center px-5 py-10 sm:px-8 sm:py-16">
      <div className={`w-full ${width}`}>{children}</div>
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
