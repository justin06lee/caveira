"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";

import { CenteredMain, Notice, SiteFooter, SiteHeader } from "@/components/chrome";
import { errorMessage, fetchMe, postJson, type Me } from "@/lib/client";

/** ABCD2345 -> ABCD-2345. The dash is display only; the API upper-cases anyway. */
function formatCode(raw: string): string {
  const cleaned = raw.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 8);
  return cleaned.length > 4 ? `${cleaned.slice(0, 4)}-${cleaned.slice(4)}` : cleaned;
}

export function DeviceApproval({ code }: { code: string | null }) {
  const router = useRouter();
  const [me, setMe] = useState<Me | null>(null);
  const [checking, setChecking] = useState(true);
  const [userCode, setUserCode] = useState(code ? formatCode(code) : "");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [approved, setApproved] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const viewer = await fetchMe();
        if (cancelled) return;
        if (!viewer) {
          // Carry the whole page, code and all, through the login round trip.
          const here = code ? `/cli?code=${encodeURIComponent(formatCode(code))}` : "/cli";
          router.replace(`/login?next=${encodeURIComponent(here)}`);
          return;
        }
        setMe(viewer);
        setChecking(false);
      } catch (err) {
        if (!cancelled) {
          setError(errorMessage(err));
          setChecking(false);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [code, router]);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      await postJson("/api/auth/device/approve", { userCode });
      setApproved(true);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <SiteHeader
        right={
          <>
            <Link href="/pricing" className="text-slate transition-colors hover:text-bone">
              Pricing
            </Link>
            {me && (
              <Link href="/account" className="text-slate transition-colors hover:text-bone">
                Account
              </Link>
            )}
          </>
        }
      />
      <CenteredMain>
        <div className="panel p-6 sm:p-8">
          {checking ? (
            <p className="font-mono text-sm text-slate">Checking your session…</p>
          ) : approved ? (
            <>
              <h1 className="text-xl font-semibold text-bone">Device approved</h1>
              <p className="mt-2 text-sm text-bone-dim">
                You can return to your terminal. It will pick up the session on its next poll.
              </p>
              <p className="mt-4 font-mono text-sm text-slate">
                Code <span className="text-bone">{userCode}</span> signed in as {me?.email}.
              </p>
              {me && !me.hasAccess && (
                <div className="mt-6 hairline pt-6">
                  <p className="text-sm text-bone-dim">
                    You do not have an active plan yet, and the CLI will ask for one next.
                  </p>
                  <Link href="/pricing" className="btn btn-primary mt-4">
                    Choose a plan
                  </Link>
                </div>
              )}
              {me?.hasAccess && (
                <Link href="/account" className="btn btn-ghost mt-6">
                  View account
                </Link>
              )}
            </>
          ) : (
            <>
              <h1 className="text-xl font-semibold text-bone">Connect your terminal</h1>
              <p className="mt-2 text-sm text-slate">
                Enter the code your CLI is showing to sign that machine in as {me?.email}.
              </p>

              <form className="mt-6 flex flex-col gap-4" onSubmit={onSubmit} noValidate>
                <div className="flex flex-col gap-1.5">
                  <label htmlFor="userCode" className="text-sm text-bone-dim">
                    Device code
                  </label>
                  <input
                    id="userCode"
                    name="userCode"
                    className="field text-center font-mono text-lg tracking-[0.35em] uppercase"
                    value={userCode}
                    onChange={(e) => setUserCode(formatCode(e.target.value))}
                    placeholder="XXXX-XXXX"
                    inputMode="text"
                    autoCapitalize="characters"
                    autoComplete="off"
                    spellCheck={false}
                    autoFocus
                    required
                    aria-describedby="code-hint"
                    disabled={busy}
                  />
                  <p id="code-hint" className="text-xs text-slate">
                    Eight characters, as printed in your terminal.
                  </p>
                </div>

                {error && <Notice tone="error">{error}</Notice>}

                <button
                  type="submit"
                  className="btn btn-primary"
                  disabled={busy || userCode.replace("-", "").length < 8}
                >
                  {busy ? "Approving…" : "Approve this device"}
                </button>
              </form>
            </>
          )}
        </div>
      </CenteredMain>
      <SiteFooter />
    </>
  );
}
