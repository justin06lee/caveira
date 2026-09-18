import { sql } from "drizzle-orm";
import { index, integer, sqliteTable, text, uniqueIndex } from "drizzle-orm/sqlite-core";

const now = sql`(unixepoch())`;

export const users = sqliteTable(
  "users",
  {
    id: text("id").primaryKey(),
    email: text("email").notNull(),
    // Null for accounts that only ever signed in through a provider. The
    // password login path treats null as "no password set" rather than
    // comparing against an empty hash.
    passwordHash: text("password_hash"),
    name: text("name"),
    stripeCustomerId: text("stripe_customer_id"),
    createdAt: integer("created_at").notNull().default(now),
  },
  (t) => [uniqueIndex("users_email_idx").on(t.email)],
);

// One table for both clients: the browser gets the token in an httpOnly
// cookie, the CLI keeps it in ~/.caveira/auth.json and sends it as a bearer.
// Knowing which kind a session is makes "sign out everywhere else" and
// per-device revocation possible later.
export const sessions = sqliteTable(
  "sessions",
  {
    token: text("token").primaryKey(),
    userId: text("user_id")
      .notNull()
      .references(() => users.id, { onDelete: "cascade" }),
    kind: text("kind", { enum: ["web", "cli"] }).notNull(),
    label: text("label"),
    createdAt: integer("created_at").notNull().default(now),
    expiresAt: integer("expires_at").notNull(),
  },
  (t) => [index("sessions_user_idx").on(t.userId)],
);

// The CLI's half of the device flow. The CLI polls on `deviceCode` (secret);
// the human types `userCode` into the browser, so it is short and unambiguous.
export const deviceCodes = sqliteTable(
  "device_codes",
  {
    deviceCode: text("device_code").primaryKey(),
    userCode: text("user_code").notNull(),
    // Set when a signed-in browser approves the code; until then the CLI polls
    // and gets "pending".
    userId: text("user_id").references(() => users.id, { onDelete: "cascade" }),
    sessionToken: text("session_token"),
    approvedAt: integer("approved_at"),
    createdAt: integer("created_at").notNull().default(now),
    expiresAt: integer("expires_at").notNull(),
  },
  (t) => [uniqueIndex("device_codes_user_code_idx").on(t.userCode)],
);

// Stripe is the source of truth for money; this row is the cached answer to
// "may this user in?" so the CLI never waits on a Stripe round trip.
export const subscriptions = sqliteTable(
  "subscriptions",
  {
    userId: text("user_id")
      .primaryKey()
      .references(() => users.id, { onDelete: "cascade" }),
    planId: text("plan_id").notNull(),
    status: text("status", {
      enum: ["active", "trialing", "past_due", "canceled", "incomplete"],
    }).notNull(),
    stripeSubscriptionId: text("stripe_subscription_id"),
    currentPeriodEnd: integer("current_period_end"),
    cancelAtPeriodEnd: integer("cancel_at_period_end", { mode: "boolean" }).notNull().default(false),
    updatedAt: integer("updated_at").notNull().default(now),
  },
);

export type User = typeof users.$inferSelect;
export type Session = typeof sessions.$inferSelect;
export type Subscription = typeof subscriptions.$inferSelect;
