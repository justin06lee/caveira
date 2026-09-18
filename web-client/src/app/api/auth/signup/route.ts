import { eq } from "drizzle-orm";
import { NextResponse, type NextRequest } from "next/server";

import { db } from "@/db";
import { users } from "@/db/schema";
import { createSession, hashPassword, newId, SESSION_COOKIE } from "@/lib/auth";
import { bad, normalizeEmail, readJson } from "@/lib/http";

export async function POST(req: NextRequest) {
  const body = await readJson<{ email?: string; password?: string; name?: string }>(req);
  const email = normalizeEmail(body?.email);
  const password = body?.password ?? "";

  if (!email) return bad("Enter a valid email address.");
  if (password.length < 8) return bad("Password must be at least 8 characters.");

  const [existing] = await db.select({ id: users.id }).from(users).where(eq(users.email, email)).limit(1);
  if (existing) return bad("That email already has an account. Log in instead.", 409);

  const id = newId("usr");
  const name = body?.name?.trim() || null;
  await db.insert(users).values({ id, email, passwordHash: await hashPassword(password), name });

  const { token, expiresAt } = await createSession(id, "web");
  const res = NextResponse.json({ user: { id, email, name } }, { status: 201 });
  res.cookies.set(SESSION_COOKIE, token, {
    httpOnly: true,
    sameSite: "lax",
    secure: process.env.NODE_ENV === "production",
    path: "/",
    expires: new Date(expiresAt * 1000),
  });
  return res;
}
