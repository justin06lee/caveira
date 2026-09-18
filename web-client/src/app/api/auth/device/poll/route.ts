import { eq } from "drizzle-orm";
import { NextResponse, type NextRequest } from "next/server";

import { db } from "@/db";
import { deviceCodes, subscriptions, users } from "@/db/schema";
import { hasAccess } from "@/lib/auth";
import { bad, readJson } from "@/lib/http";

export async function POST(req: NextRequest) {
  const body = await readJson<{ deviceCode?: string }>(req);
  if (!body?.deviceCode) return bad("deviceCode is required.");

  const [pending] = await db
    .select()
    .from(deviceCodes)
    .where(eq(deviceCodes.deviceCode, body.deviceCode))
    .limit(1);
  if (!pending) return NextResponse.json({ status: "expired" }, { status: 404 });

  if (pending.expiresAt <= Math.floor(Date.now() / 1000)) {
    await db.delete(deviceCodes).where(eq(deviceCodes.deviceCode, body.deviceCode));
    return NextResponse.json({ status: "expired" }, { status: 410 });
  }

  if (!pending.approvedAt || !pending.sessionToken || !pending.userId) {
    return NextResponse.json({ status: "pending", userCode: pending.userCode });
  }

  const [user] = await db.select().from(users).where(eq(users.id, pending.userId)).limit(1);
  const [subscription] = await db
    .select()
    .from(subscriptions)
    .where(eq(subscriptions.userId, pending.userId))
    .limit(1);

  // One-shot: the token is handed over exactly once, so a device code that
  // leaks after the fact is worth nothing.
  await db.delete(deviceCodes).where(eq(deviceCodes.deviceCode, body.deviceCode));

  return NextResponse.json({
    status: "approved",
    token: pending.sessionToken,
    user: user ? { id: user.id, email: user.email, name: user.name } : null,
    subscription: subscription
      ? { planId: subscription.planId, status: subscription.status }
      : null,
    hasAccess: hasAccess(subscription ?? null),
  });
}
