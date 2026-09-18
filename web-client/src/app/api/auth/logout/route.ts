import { eq } from "drizzle-orm";
import { NextResponse, type NextRequest } from "next/server";

import { db } from "@/db";
import { sessions } from "@/db/schema";
import { SESSION_COOKIE } from "@/lib/auth";

// Revokes whichever session made the call: the browser's cookie, or the CLI's
// bearer token when it signs out, so a copied token dies with the sign-out.
export async function POST(req: NextRequest) {
  const bearer = req.headers.get("authorization")?.replace(/^Bearer\s+/i, "");
  const token = bearer || req.cookies.get(SESSION_COOKIE)?.value;
  if (token) await db.delete(sessions).where(eq(sessions.token, token));
  const res = NextResponse.json({ ok: true });
  res.cookies.delete(SESSION_COOKIE);
  return res;
}
