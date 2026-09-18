import { and, eq, isNull } from "drizzle-orm";
import { NextResponse, type NextRequest } from "next/server";

import { db } from "@/db";
import { deviceCodes } from "@/db/schema";
import { createSession, getViewer } from "@/lib/auth";
import { bad, readJson } from "@/lib/http";

// The browser half: a signed-in user vouches for a code the CLI is waiting on.
export async function POST(req: NextRequest) {
  const viewer = await getViewer(req);
  if (!viewer) return bad("Sign in first.", 401);

  const body = await readJson<{ userCode?: string }>(req);
  const userCode = body?.userCode?.trim().toUpperCase();
  if (!userCode) return bad("Enter the code shown in your terminal.");

  const [pending] = await db
    .select()
    .from(deviceCodes)
    .where(and(eq(deviceCodes.userCode, userCode), isNull(deviceCodes.approvedAt)))
    .limit(1);
  if (!pending) return bad("That code is not waiting for approval. Check the terminal for a fresh one.", 404);
  if (pending.expiresAt <= Math.floor(Date.now() / 1000)) {
    await db.delete(deviceCodes).where(eq(deviceCodes.deviceCode, pending.deviceCode));
    return bad("That code expired. Start again in your terminal.", 410);
  }

  const { token } = await createSession(viewer.user.id, "cli", "CLI");
  await db
    .update(deviceCodes)
    .set({ userId: viewer.user.id, sessionToken: token, approvedAt: Math.floor(Date.now() / 1000) })
    .where(eq(deviceCodes.deviceCode, pending.deviceCode));

  return NextResponse.json({ ok: true });
}
