import { NextResponse } from "next/server";

import { PLANS } from "@/lib/plans";
import { stripeEnabled } from "@/lib/stripe";

// Public: the TUI draws its plan cards from this, and so does the pricing page.
export async function GET() {
  return NextResponse.json({
    plans: PLANS.map(({ id, name, tagline, priceUsd, interval, features }) => ({
      id,
      name,
      tagline,
      priceUsd,
      interval,
      features,
    })),
    billing: stripeEnabled ? "stripe" : "dev",
  });
}
