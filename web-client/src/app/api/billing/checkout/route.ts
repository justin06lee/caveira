import { eq } from "drizzle-orm";
import { NextResponse, type NextRequest } from "next/server";

import { db } from "@/db";
import { subscriptions, users } from "@/db/schema";
import { getViewer } from "@/lib/auth";
import { bad, readJson } from "@/lib/http";
import { planById, plansWithPrices } from "@/lib/plans";
import { appUrl, stripe, stripeEnabled } from "@/lib/stripe";

// Serves both clients: the website redirects to `url`, the TUI opens it in the
// user's browser. In dev billing mode there is no url and the subscription is
// written on the spot.
export async function POST(req: NextRequest) {
  const viewer = await getViewer(req);
  if (!viewer) return bad("Sign in first.", 401);

  const body = await readJson<{ planId?: string }>(req);
  const plan = body?.planId ? planById(body.planId) : undefined;
  if (!plan) return bad("Unknown plan.");

  if (!stripeEnabled || !stripe) {
    await db
      .insert(subscriptions)
      .values({
        userId: viewer.user.id,
        planId: plan.id,
        status: "active",
        currentPeriodEnd: Math.floor(Date.now() / 1000) + 30 * 86400,
        updatedAt: Math.floor(Date.now() / 1000),
      })
      .onConflictDoUpdate({
        target: subscriptions.userId,
        set: { planId: plan.id, status: "active", updatedAt: Math.floor(Date.now() / 1000) },
      });
    return NextResponse.json({ mode: "dev", planId: plan.id, activated: true });
  }

  const priceId = plansWithPrices().find((p) => p.id === plan.id)?.stripePriceId;
  if (!priceId) {
    return bad(`No Stripe price configured for ${plan.name}. Set STRIPE_PRICE_${plan.id.toUpperCase()}.`, 500);
  }

  let customerId = viewer.user.stripeCustomerId;
  if (!customerId) {
    const customer = await stripe.customers.create({
      email: viewer.user.email,
      name: viewer.user.name ?? undefined,
      metadata: { userId: viewer.user.id },
    });
    customerId = customer.id;
    await db.update(users).set({ stripeCustomerId: customerId }).where(eq(users.id, viewer.user.id));
  }

  const session = await stripe.checkout.sessions.create({
    mode: "subscription",
    customer: customerId,
    line_items: [{ price: priceId, quantity: 1 }],
    success_url: `${appUrl()}/account?checkout=success`,
    cancel_url: `${appUrl()}/pricing?checkout=cancelled`,
    // The webhook reads these back: the Stripe objects alone do not say which
    // caveira user or plan a subscription belongs to.
    metadata: { userId: viewer.user.id, planId: plan.id },
    subscription_data: { metadata: { userId: viewer.user.id, planId: plan.id } },
  });

  return NextResponse.json({ mode: "stripe", url: session.url });
}
