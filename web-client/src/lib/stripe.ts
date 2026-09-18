import Stripe from "stripe";

// Billing is optional in development: without a secret key the app runs in dev
// billing mode, where "subscribe" writes the subscription row directly. That
// keeps the whole signup -> plan -> access path runnable before anyone has a
// Stripe account, and the real path is the same code with a key present.
export const stripeEnabled = Boolean(process.env.STRIPE_SECRET_KEY);

export const stripe = stripeEnabled
  ? new Stripe(process.env.STRIPE_SECRET_KEY as string)
  : null;

export function appUrl(): string {
  return process.env.APP_URL ?? "http://localhost:3000";
}
