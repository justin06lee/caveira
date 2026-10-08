import { Check } from "lucide-react";
import { subscribe, useStore } from "../lib/store";
import type { Plan } from "../lib/types";

// Plans is the weight classes to pick from: Free across the top, the
// three middle ones side by side, Champion across the bottom. The one
// this install is on is marked. locked, in a chat, is the model the plan
// lacks; a plan without it cannot be picked there.
export function Plans({ locked, held }: { locked?: string; held?: boolean }) {
  const plans = useStore((s) => s.plans);
  const current = useStore((s) => s.plan);
  if (!plans.length) return null;
  const free = plans.find((p) => p.price === 0);
  const top = plans[plans.length - 1];
  const middle = plans.filter((p) => p !== free && p !== top);

  const props = (p: Plan) => ({
    plan: p,
    current: p.id === current,
    runs: !locked || p.large,
    held: Boolean(held),
    subscribed: Boolean(current),
  });

  return (
    <div className="plans">
      {free && <PlanCard {...props(free)} wide />}
      <div className="plans-row">
        {middle.map((p) => (
          <PlanCard key={p.id} {...props(p)} />
        ))}
      </div>
      {top !== free && <PlanCard {...props(top)} wide champion />}
    </div>
  );
}

function PlanCard({
  plan,
  current,
  runs,
  held,
  subscribed,
  wide,
  champion,
}: {
  plan: Plan;
  current: boolean;
  runs: boolean;
  held: boolean;
  subscribed: boolean;
  wide?: boolean;
  champion?: boolean;
}) {
  // A plan already on that can run what is held sends it.
  const label = current ? (held && runs ? "Send it" : "Current") : plan.price || subscribed ? "Choose" : "Start free";
  const button = (
    <button
      className={`btn plan-btn${champion ? " on-dark" : plan.price ? " primary" : ""}`}
      disabled={!runs || (current && !held)}
      onClick={() => subscribe(plan.id)}
    >
      {current && !held && <Check size={13} />}
      {label}
    </button>
  );
  const price = (
    <div className="plan-price">
      {plan.price ? (
        <>
          <b>${plan.price}</b>
          <span> / month</span>
        </>
      ) : (
        <b>Free</b>
      )}
    </div>
  );

  return (
    <div
      className={`plan${wide ? " wide" : ""}${champion ? " champion" : ""}${current ? " current" : ""}${runs ? "" : " short"}`}
    >
      {wide ? (
        <>
          <div className="plan-name">{plan.name}</div>
          <div className="plan-lines grow">{plan.lines.join(" · ")}</div>
          <Usage n={plan.usage} />
          {price}
          {button}
        </>
      ) : (
        <>
          <div className="plan-name">{plan.name}</div>
          {price}
          <ul className="plan-lines">
            {plan.lines.map((l) => (
              <li key={l}>{l}</li>
            ))}
          </ul>
          <div className="plan-foot">
            <Usage n={plan.usage} />
            {button}
          </div>
        </>
      )}
    </div>
  );
}

// Usage is how much a plan allows, as five bars of rising height.
function Usage({ n }: { n: number }) {
  return (
    <svg className="usage" width="29" height="14" viewBox="0 0 29 14" aria-label={`Usage ${n} of 5`}>
      {[0, 1, 2, 3, 4].map((i) => {
        const h = 4 + i * 2.5;
        return <rect key={i} x={i * 6} y={14 - h} width="4" height={h} rx="1" className={i < n ? "on" : ""} />;
      })}
    </svg>
  );
}
