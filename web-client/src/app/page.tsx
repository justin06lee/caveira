import Link from "next/link";

import { SiteFooter, SiteHeader } from "@/components/chrome";

export default function Home() {
  return (
    <>
      <SiteHeader />
      <main className="flex flex-1 items-center px-5 py-12 sm:px-8 sm:py-20">
        <div className="mx-auto w-full max-w-3xl">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src="/caveira.svg"
            alt="caveira"
            width={960}
            height={360}
            className="w-full rounded-xl border border-edge"
          />

          <h1 className="sr-only">caveira</h1>

          <p className="mt-8 text-lg leading-relaxed text-bone sm:text-xl">
            A Claude Code-style coding agent for abliterated models.{" "}
            <span className="text-bone-dim">
              Your model, your machine, no refusals.
            </span>
          </p>

          <div className="mt-8 flex flex-col gap-3 sm:flex-row sm:items-center">
            <Link href="/pricing" className="btn btn-primary">
              See pricing
            </Link>
            <Link href="/login" className="btn btn-ghost">
              Log in
            </Link>
          </div>

          <p className="mt-10 font-mono text-xs text-slate">
            Already at a terminal? Pair it from{" "}
            <Link href="/cli" className="link">
              /cli
            </Link>
            .
          </p>
        </div>
      </main>
      <SiteFooter />
    </>
  );
}
