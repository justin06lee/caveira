import { NextResponse, type NextRequest } from "next/server";

import { getViewer } from "@/lib/auth";
import { bad } from "@/lib/http";
import { appUrl, stripe, stripeEnabled } from "@/lib/stripe";

export async function POST(req: NextRequest) {
  const viewer = await getViewer(req);
  if (!viewer) return bad("Sign in first.", 401);
  if (!stripeEnabled || !stripe) return bad("Billing is running in dev mode; there is no portal.", 501);
  if (!viewer.user.stripeCustomerId) return bad("No billing account yet. Pick a plan first.", 400);

  const session = await stripe.billingPortal.sessions.create({
    customer: viewer.user.stripeCustomerId,
    return_url: `${appUrl()}/account`,
  });
  return NextResponse.json({ url: session.url });
}
