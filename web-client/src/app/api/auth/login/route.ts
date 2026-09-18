import { eq } from "drizzle-orm";
import { NextResponse, type NextRequest } from "next/server";

import { db } from "@/db";
import { users } from "@/db/schema";
import { createSession, SESSION_COOKIE, verifyPassword } from "@/lib/auth";
import { bad, normalizeEmail, readJson } from "@/lib/http";

export async function POST(req: NextRequest) {
  const body = await readJson<{ email?: string; password?: string }>(req);
  const email = normalizeEmail(body?.email);
  if (!email || !body?.password) return bad("Email and password are required.");

  const [user] = await db.select().from(users).where(eq(users.email, email)).limit(1);
  // Same message either way: distinguishing "no such account" from "wrong
  // password" tells a stranger which emails are registered.
  if (!user || !(await verifyPassword(body.password, user.passwordHash))) {
    return bad("Incorrect email or password.", 401);
  }

  const { token, expiresAt } = await createSession(user.id, "web");
  const res = NextResponse.json({ user: { id: user.id, email: user.email, name: user.name } });
  res.cookies.set(SESSION_COOKIE, token, {
    httpOnly: true,
    sameSite: "lax",
    secure: process.env.NODE_ENV === "production",
    path: "/",
    expires: new Date(expiresAt * 1000),
  });
  return res;
}
