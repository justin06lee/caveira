import { randomBytes, randomInt, scrypt as scryptCb, timingSafeEqual } from "node:crypto";
import { promisify } from "node:util";

import { eq } from "drizzle-orm";
import type { NextRequest } from "next/server";

import { db } from "@/db";
import { sessions, subscriptions, users, type Subscription, type User } from "@/db/schema";

const scrypt = promisify(scryptCb);

export const SESSION_COOKIE = "caveira_session";
const WEB_SESSION_DAYS = 30;
const CLI_SESSION_DAYS = 365;

export async function hashPassword(password: string): Promise<string> {
  const salt = randomBytes(16);
  const derived = (await scrypt(password, salt, 64)) as Buffer;
  return `scrypt$${salt.toString("hex")}$${derived.toString("hex")}`;
}

export async function verifyPassword(password: string, stored: string | null): Promise<boolean> {
  if (!stored) return false;
  const [scheme, saltHex, hashHex] = stored.split("$");
  if (scheme !== "scrypt" || !saltHex || !hashHex) return false;
  const expected = Buffer.from(hashHex, "hex");
  const derived = (await scrypt(password, Buffer.from(saltHex, "hex"), expected.length)) as Buffer;
  return timingSafeEqual(expected, derived);
}

export function newId(prefix: string): string {
  return `${prefix}_${randomBytes(12).toString("hex")}`;
}

export function newToken(): string {
  return randomBytes(32).toString("base64url");
}

// No I, O, 0 or 1: the user code gets read aloud off one screen and typed into
// another, and those four are where that goes wrong.
const CODE_ALPHABET = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789";

export function newUserCode(): string {
  let code = "";
  for (let i = 0; i < 8; i++) {
    if (i === 4) code += "-";
    code += CODE_ALPHABET[randomInt(CODE_ALPHABET.length)];
  }
  return code;
}

export async function createSession(userId: string, kind: "web" | "cli", label?: string) {
  const token = newToken();
  const days = kind === "cli" ? CLI_SESSION_DAYS : WEB_SESSION_DAYS;
  const expiresAt = Math.floor(Date.now() / 1000) + days * 86400;
  await db.insert(sessions).values({ token, userId, kind, label, expiresAt });
  return { token, expiresAt };
}

export type Viewer = { user: User; subscription: Subscription | null };

// Accepts either half of the split: the browser's cookie or the CLI's bearer
// token. Every protected route goes through here so the two clients cannot
// drift apart on what counts as signed in.
export async function getViewer(req: NextRequest): Promise<Viewer | null> {
  const bearer = req.headers.get("authorization")?.replace(/^Bearer\s+/i, "");
  const token = bearer || req.cookies.get(SESSION_COOKIE)?.value;
  if (!token) return null;

  const [row] = await db
    .select({ session: sessions, user: users })
    .from(sessions)
    .innerJoin(users, eq(users.id, sessions.userId))
    .where(eq(sessions.token, token))
    .limit(1);
  if (!row) return null;

  if (row.session.expiresAt <= Math.floor(Date.now() / 1000)) {
    await db.delete(sessions).where(eq(sessions.token, token));
    return null;
  }

  const [subscription] = await db
    .select()
    .from(subscriptions)
    .where(eq(subscriptions.userId, row.user.id))
    .limit(1);

  return { user: row.user, subscription: subscription ?? null };
}

// What the gate actually asks. Past due still gets in: Stripe retries for days
// and locking someone out mid-retry punishes an expired card, not a freeloader.
export function hasAccess(subscription: Subscription | null): boolean {
  return (
    subscription !== null &&
    (subscription.status === "active" ||
      subscription.status === "trialing" ||
      subscription.status === "past_due")
  );
}

export function publicUser(viewer: Viewer) {
  return {
    id: viewer.user.id,
    email: viewer.user.email,
    name: viewer.user.name,
    subscription: viewer.subscription
      ? {
          planId: viewer.subscription.planId,
          status: viewer.subscription.status,
          currentPeriodEnd: viewer.subscription.currentPeriodEnd,
          cancelAtPeriodEnd: viewer.subscription.cancelAtPeriodEnd,
        }
      : null,
    hasAccess: hasAccess(viewer.subscription),
  };
}
