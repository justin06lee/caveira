import { eq } from "drizzle-orm";
import { NextResponse } from "next/server";
import type Stripe from "stripe";

import { db } from "@/db";
import { subscriptions, users } from "@/db/schema";
import { PLANS } from "@/lib/plans";
import { stripe, stripeEnabled } from "@/lib/stripe";

// Stripe decides what is true about money; this route is how that truth
// reaches the database the gate reads.
export async function POST(req: Request) {
  if (!stripeEnabled || !stripe) return NextResponse.json({ error: "Stripe is not configured." }, { status: 501 });

  const secret = process.env.STRIPE_WEBHOOK_SECRET;
  if (!secret) return NextResponse.json({ error: "STRIPE_WEBHOOK_SECRET is not set." }, { status: 500 });

  const signature = req.headers.get("stripe-signature");
  if (!signature) return NextResponse.json({ error: "Missing signature." }, { status: 400 });

  // Raw body: the signature is over the exact bytes Stripe sent.
  const payload = await req.text();
  let event: Stripe.Event;
  try {
    event = await stripe.webhooks.constructEventAsync(payload, signature, secret);
  } catch (err) {
    return NextResponse.json({ error: `Bad signature: ${(err as Error).message}` }, { status: 400 });
  }

  switch (event.type) {
    case "customer.subscription.created":
    case "customer.subscription.updated":
    case "customer.subscription.deleted":
      await syncSubscription(event.data.object);
      break;
    default:
      break;
  }

  return NextResponse.json({ received: true });
}

async function syncSubscription(sub: Stripe.Subscription) {
  const userId = await resolveUserId(sub);
  if (!userId) return;

  const item = sub.items.data[0];
  const planId =
    (sub.metadata?.planId as string | undefined) ??
    PLANS.find((p) => p.id === (item?.price.lookup_key ?? ""))?.id ??
    planIdForPrice(item?.price.id) ??
    "operator";

  const status = sub.status === "unpaid" ? "past_due" : sub.status;
  const now = Math.floor(Date.now() / 1000);

  await db
    .insert(subscriptions)
    .values({
      userId,
      planId,
      status: status as "active" | "trialing" | "past_due" | "canceled" | "incomplete",
      stripeSubscriptionId: sub.id,
      currentPeriodEnd: item?.current_period_end ?? null,
      cancelAtPeriodEnd: sub.cancel_at_period_end,
      updatedAt: now,
    })
    .onConflictDoUpdate({
      target: subscriptions.userId,
      set: {
        planId,
        status: status as "active" | "trialing" | "past_due" | "canceled" | "incomplete",
        stripeSubscriptionId: sub.id,
        currentPeriodEnd: item?.current_period_end ?? null,
        cancelAtPeriodEnd: sub.cancel_at_period_end,
        updatedAt: now,
      },
    });
}

// Metadata is set at checkout, but a subscription created by hand in the
// Stripe dashboard has none — fall back to the customer id we stored.
async function resolveUserId(sub: Stripe.Subscription): Promise<string | null> {
  const fromMetadata = sub.metadata?.userId;
  if (fromMetadata) return fromMetadata;

  const customerId = typeof sub.customer === "string" ? sub.customer : sub.customer?.id;
  if (!customerId) return null;

  const [user] = await db
    .select({ id: users.id })
    .from(users)
    .where(eq(users.stripeCustomerId, customerId))
    .limit(1);
  return user?.id ?? null;
}

function planIdForPrice(priceId: string | undefined): string | undefined {
  if (!priceId) return undefined;
  return PLANS.find((p) => process.env[`STRIPE_PRICE_${p.id.toUpperCase()}`] === priceId)?.id;
}
