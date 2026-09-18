"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { Notice, SiteFooter, SiteHeader } from "@/components/chrome";
import {
  errorMessage,
  fetchMe,
  getJson,
  postJson,
  type CheckoutResponse,
  type Me,
  type PlanCard,
  type PlansResponse,
} from "@/lib/client";

const RECOMMENDED = "operator";

export function PricingBoard({ checkout }: { checkout: string | null }) {
  const router = useRouter();
  const [plans, setPlans] = useState<PlanCard[] | null>(null);
  const [billing, setBilling] = useState<"stripe" | "dev">("stripe");
  const [me, setMe] = useState<Me | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [pending, setPending] = useState<string | null>(null);
  const [activated, setActivated] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [catalogue, viewer] = await Promise.all([
          getJson<PlansResponse>("/api/plans"),
          fetchMe(),
        ]);
        if (cancelled) return;
        setPlans(catalogue.plans);
        setBilling(catalogue.billing);
        setMe(viewer);
      } catch (err) {
        if (!cancelled) setLoadError(errorMessage(err));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  async function choose(planId: string) {
    if (pending) return;
    setPending(planId);
    setActionError(null);
    try {
      const result = await postJson<CheckoutResponse>("/api/billing/checkout", {
        planId,
      });
      if (result.mode === "stripe") {
        window.location.href = result.url;
        return;
      }
      // Dev billing: the subscription is already written, so say so and move on.
      setActivated(planId);
      setTimeout(() => router.push("/account"), 900);
    } catch (err) {
      setActionError(errorMessage(err));
      setPending(null);
    }
  }

  const currentPlanId = me?.hasAccess ? me.subscription?.planId : undefined;

  return (
    <>
      <SiteHeader
        right={
          me ? (
            <Link href="/account" className="text-slate transition-colors hover:text-bone">
              Account
            </Link>
          ) : (
            <>
              <Link href="/login" className="text-slate transition-colors hover:text-bone">
                Log in
              </Link>
              <Link href="/signup?next=%2Fpricing" className="text-slate transition-colors hover:text-bone">
                Sign up
              </Link>
            </>
          )
        }
      />

      <main className="flex-1 px-5 py-12 sm:px-8 sm:py-16">
        <div className="mx-auto w-full max-w-5xl">
          <h1 className="text-2xl font-semibold text-bone sm:text-3xl">Plans</h1>
          <p className="mt-2 max-w-xl text-sm text-slate sm:text-base">
            One agent, three sizes. Cancel whenever; the model stays on your machine either way.
          </p>

          <div className="mt-6 flex flex-col gap-3">
            {checkout === "cancelled" && (
              <Notice tone="info">Checkout cancelled. Nothing was charged.</Notice>
            )}
            {billing === "dev" && (
              <Notice tone="info">
                Billing is running in dev mode: choosing a plan activates it immediately, with no payment.
              </Notice>
            )}
            {loadError && <Notice tone="error">{loadError}</Notice>}
            {actionError && <Notice tone="error">{actionError}</Notice>}
            {activated && <Notice tone="success">Plan activated. Taking you to your account…</Notice>}
          </div>

          {!plans && !loadError && (
            <p className="mt-10 font-mono text-sm text-slate">Loading plans…</p>
          )}

          {plans && (
            <div className="mt-8 grid grid-cols-1 gap-5 md:grid-cols-3">
              {plans.map((plan) => {
                const recommended = plan.id === RECOMMENDED;
                const isCurrent = plan.id === currentPlanId;
                return (
                  <section
                    key={plan.id}
                    aria-labelledby={`plan-${plan.id}`}
                    className={`panel flex flex-col p-6 ${
                      recommended ? "border-ember/60 md:-mt-3 md:pb-9" : ""
                    }`}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <h2 id={`plan-${plan.id}`} className="text-lg font-semibold text-bone">
                        {plan.name}
                      </h2>
                      {recommended && (
                        <span className="rounded-full border border-ember/50 px-2 py-0.5 font-mono text-[10px] tracking-[0.2em] text-ember uppercase">
                          Recommended
                        </span>
                      )}
                      {isCurrent && !recommended && (
                        <span className="rounded-full border border-brass/50 px-2 py-0.5 font-mono text-[10px] tracking-[0.2em] text-brass uppercase">
                          Current
                        </span>
                      )}
                    </div>

                    {isCurrent && recommended && (
                      <span className="mt-2 self-start rounded-full border border-brass/50 px-2 py-0.5 font-mono text-[10px] tracking-[0.2em] text-brass uppercase">
                        Current
                      </span>
                    )}

                    <p className="mt-2 text-sm text-slate">{plan.tagline}</p>

                    <p className="mt-5 flex items-baseline gap-1.5">
                      <span className="font-mono text-3xl text-bone">${plan.priceUsd}</span>
                      <span className="text-sm text-slate">/ {plan.interval}</span>
                    </p>

                    <ul className="mt-5 flex flex-col gap-2 text-sm text-bone-dim">
                      {plan.features.map((feature) => (
                        <li key={feature} className="flex gap-2">
                          <span aria-hidden="true" className="mt-2 h-1 w-1 shrink-0 rounded-[1px] bg-slate-deep" />
                          {feature}
                        </li>
                      ))}
                    </ul>

                    <div className="mt-auto pt-7">
                      {!me ? (
                        <Link
                          href="/signup?next=%2Fpricing"
                          className={`btn w-full ${recommended ? "btn-primary" : "btn-ghost"}`}
                        >
                          Get {plan.name}
                        </Link>
                      ) : isCurrent ? (
                        <button type="button" className="btn btn-ghost w-full" disabled>
                          Current plan
                        </button>
                      ) : (
                        <button
                          type="button"
                          className={`btn w-full ${recommended ? "btn-primary" : "btn-ghost"}`}
                          onClick={() => choose(plan.id)}
                          disabled={pending !== null}
                        >
                          {pending === plan.id ? "Working…" : currentPlanId ? `Switch to ${plan.name}` : `Choose ${plan.name}`}
                        </button>
                      )}
                    </div>
                  </section>
                );
              })}
            </div>
          )}
        </div>
      </main>
      <SiteFooter />
    </>
  );
}
