import { Check, TrendingUp } from "lucide-react";
import { subscribe, useStore } from "../lib/store";
import type { Plan } from "../lib/types";

// Plans is the weight classes, laid out as Toji's pricing page lays out
// its own: Free across the top, the three middle weights side by side,
// Champion across the bottom. locked, in a chat, is the model the plan
// lacks; a plan without it cannot be picked there. held says a message
// is waiting on the pick.
export function Plans({ locked, held }: { locked?: string; held?: boolean }) {
  const plans = useStore((s) => s.plans);
  const current = useStore((s) => s.plan);
  if (!plans.length) return null;
  const free = plans.find((p) => p.price === 0);
  const top = plans[plans.length - 1];
  const middle = plans.filter((p) => p !== free && p !== top);

  const state = (p: Plan): CardState => ({
    plan: p,
    current: p.id === current,
    runs: !locked || p.large,
    held: Boolean(held),
    subscribed: Boolean(current),
    locked,
  });

  return (
    <div className="plans">
      {free && <WideCard {...state(free)} />}
      <div className="plans-grid">
        {middle.map((p) => (
          <PlanCard key={p.id} {...state(p)} />
        ))}
      </div>
      {top !== free && <WideCard {...state(top)} />}
      <p className="plans-note">Billing isn&rsquo;t live yet, so no plan charges anything for now.</p>
    </div>
  );
}

interface CardState {
  plan: Plan;
  current: boolean;
  runs: boolean;
  held: boolean;
  subscribed: boolean;
  locked?: string;
}

function Name({ plan }: { plan: Plan }) {
  return (
    <div className="plan-name">
      <h3>{plan.name}</h3>
      {plan.popular && (
        <span className="plan-pill">
          <TrendingUp size={10} /> Popular
        </span>
      )}
    </div>
  );
}

function Price({ plan }: { plan: Plan }) {
  return (
    <p className="plan-price">
      <b>${plan.price}</b>
      {plan.price > 0 && <span>/month</span>}
    </p>
  );
}

function Features({ plan }: { plan: Plan }) {
  return (
    <ul className="plan-features">
      {plan.features.map((f) => (
        <li key={f}>
          <Check size={14} />
          <span>{f}</span>
        </li>
      ))}
    </ul>
  );
}

function Action({ plan, current, runs, held, subscribed, locked }: CardState) {
  // The plan already on, when it can run what is held, sends it.
  if (current && held && runs) {
    return (
      <button className="plan-btn primary" onClick={() => subscribe(plan.id)}>
        Send it
      </button>
    );
  }
  if (current) {
    return (
      <button className="plan-btn quiet" disabled>
        <Check size={14} /> Current plan
      </button>
    );
  }
  if (!runs) {
    return (
      <button className="plan-btn quiet" disabled>
        No {locked}
      </button>
    );
  }
  return (
    <button className={`plan-btn${plan.popular ? " primary" : ""}`} onClick={() => subscribe(plan.id)}>
      {plan.price ? `Choose ${plan.name}` : subscribed ? "Switch to Free" : "Start free"}
    </button>
  );
}

// PlanCard is a column of the grid. Every card spans the same four rows
// (name, price, features, button), so a short list cannot pull one
// card's button out of line with its neighbours'.
function PlanCard(s: CardState) {
  return (
    <div className={`plan-card${s.plan.popular ? " popular" : ""}${s.runs ? "" : " short"}`}>
      <Name plan={s.plan} />
      <Price plan={s.plan} />
      <Features plan={s.plan} />
      <Action {...s} />
    </div>
  );
}

// WideCard is a plan across the whole width: name and price, what it
// comes with, the button.
function WideCard(s: CardState) {
  return (
    <div className={`plan-card wide${s.runs ? "" : " short"}`}>
      <div>
        <Name plan={s.plan} />
        <Price plan={s.plan} />
      </div>
      <Features plan={s.plan} />
      <Action {...s} />
    </div>
  );
}
