// The plan catalogue lives in code, not the database: it is content, it needs
// to be rendered identically by the website and the TUI, and a price change is
// a deploy either way. The database only stores which plan a user is on.
export type Plan = {
  id: string;
  name: string;
  tagline: string;
  priceUsd: number;
  interval: "month";
  features: string[];
  // Filled from the environment because price ids differ between a Stripe
  // sandbox and the live account.
  stripePriceId?: string;
};

export const PLANS: Plan[] = [
  {
    id: "recruit",
    name: "Recruit",
    tagline: "Kick the tires on your own hardware.",
    priceUsd: 12,
    interval: "month",
    features: ["1 machine", "Local models only", "Community support"],
  },
  {
    id: "operator",
    name: "Operator",
    tagline: "The everyday plan for real work.",
    priceUsd: 29,
    interval: "month",
    features: ["5 machines", "Local and hosted models", "Session history", "Email support"],
  },
  {
    id: "comando",
    name: "Comando",
    tagline: "Everything, for people who live in the terminal.",
    priceUsd: 79,
    interval: "month",
    features: ["Unlimited machines", "Priority model access", "Shared team workspaces", "Direct support"],
  },
];

export function planById(id: string): Plan | undefined {
  return PLANS.find((p) => p.id === id);
}

export function plansWithPrices(): Plan[] {
  return PLANS.map((p) => ({
    ...p,
    stripePriceId: process.env[`STRIPE_PRICE_${p.id.toUpperCase()}`],
  }));
}
