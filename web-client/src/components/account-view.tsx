"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { CenteredMain, Notice, SiteFooter, SiteHeader } from "@/components/chrome";
import {
  errorMessage,
  fetchMe,
  formatDate,
  formatStatus,
  getJson,
  postJson,
  type Me,
  type PlansResponse,
} from "@/lib/client";

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1 py-3 sm:flex-row sm:items-baseline sm:justify-between sm:gap-6">
      <dt className="text-sm text-slate">{label}</dt>
      <dd className="text-sm text-bone sm:text-right">{children}</dd>
    </div>
  );
}

export function AccountView({ checkout }: { checkout: string | null }) {
  const router = useRouter();
  const [me, setMe] = useState<Me | null>(null);
  const [planNames, setPlanNames] = useState<Record<string, string>>({});
  const [billing, setBilling] = useState<"stripe" | "dev">("stripe");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<"portal" | "logout" | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [viewer, catalogue] = await Promise.all([
          fetchMe(),
          getJson<PlansResponse>("/api/plans"),
        ]);
        if (cancelled) return;
        if (!viewer) {
          router.replace("/login?next=%2Faccount");
          return;
        }
        setMe(viewer);
        setBilling(catalogue.billing);
        setPlanNames(
          Object.fromEntries(catalogue.plans.map((p) => [p.id, p.name])),
        );
        setLoading(false);
      } catch (err) {
        if (!cancelled) {
          setError(errorMessage(err));
          setLoading(false);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [router]);

  async function openPortal() {
    if (busy) return;
    setBusy("portal");
    setError(null);
    try {
      const { url } = await postJson<{ url: string }>("/api/billing/portal");
      window.location.href = url;
    } catch (err) {
      setError(errorMessage(err));
      setBusy(null);
    }
  }

  async function signOut() {
    if (busy) return;
    setBusy("logout");
    try {
      await postJson("/api/auth/logout");
      router.replace("/");
      router.refresh();
    } catch (err) {
      setError(errorMessage(err));
      setBusy(null);
    }
  }

  const subscription = me?.subscription ?? null;
  const planName = subscription
    ? planNames[subscription.planId] ?? subscription.planId
    : null;

  return (
    <>
      <SiteHeader
        right={
          <Link href="/pricing" className="text-slate transition-colors hover:text-bone">
            Pricing
          </Link>
        }
      />
      <CenteredMain width="max-w-xl">
        {loading ? (
          <p className="font-mono text-sm text-slate">Loading your account…</p>
        ) : (
          <div className="flex flex-col gap-5">
            {checkout === "success" && (
              <Notice tone="success">Subscription confirmed. Welcome aboard.</Notice>
            )}
            {error && <Notice tone="error">{error}</Notice>}

            <div className="panel p-6 sm:p-8">
              <h1 className="text-xl font-semibold text-bone">Account</h1>
              <p className="mt-1 font-mono text-sm text-slate">{me?.email}</p>

              <dl className="mt-6 divide-y divide-edge border-t border-edge">
                {me?.name && <Row label="Name">{me.name}</Row>}
                <Row label="Plan">
                  {planName ? (
                    planName
                  ) : (
                    <span className="text-bone-dim">No plan yet</span>
                  )}
                </Row>
                <Row label="Status">
                  {subscription ? (
                    <span className={me?.hasAccess ? "text-bone" : "text-ember"}>
                      {formatStatus(subscription.status)}
                    </span>
                  ) : (
                    <span className="text-bone-dim">Inactive</span>
                  )}
                </Row>
                <Row
                  label={subscription?.cancelAtPeriodEnd ? "Access ends" : "Renews"}
                >
                  <span className="font-mono">
                    {formatDate(subscription?.currentPeriodEnd)}
                  </span>
                </Row>
              </dl>

              {!subscription && (
                <p className="mt-6 text-sm text-bone-dim">
                  Pick a plan to unlock the agent on your machines.
                </p>
              )}

              <div className="mt-7 flex flex-col gap-3 sm:flex-row">
                <Link href="/pricing" className="btn btn-ghost">
                  {subscription ? "Change plan" : "Choose a plan"}
                </Link>
                {billing === "stripe" && (
                  <button
                    type="button"
                    className="btn btn-ghost"
                    onClick={openPortal}
                    disabled={busy !== null}
                  >
                    {busy === "portal" ? "Opening…" : "Manage billing"}
                  </button>
                )}
              </div>

              {billing === "dev" && (
                <p className="mt-4 text-xs text-slate">
                  Billing is in dev mode, so there is no payment portal to manage.
                </p>
              )}
            </div>

            <div className="flex items-center justify-between gap-4">
              <Link href="/cli" className="text-sm text-slate transition-colors hover:text-bone">
                Connect a terminal
              </Link>
              <button
                type="button"
                className="btn btn-quiet text-sm"
                onClick={signOut}
                disabled={busy !== null}
              >
                {busy === "logout" ? "Signing out…" : "Sign out"}
              </button>
            </div>
          </div>
        )}
      </CenteredMain>
      <SiteFooter />
    </>
  );
}
