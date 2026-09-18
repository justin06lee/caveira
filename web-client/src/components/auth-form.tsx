"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Notice } from "@/components/chrome";
import {
  destinationAfterAuth,
  errorMessage,
  fetchMe,
  postJson,
  safeNext,
} from "@/lib/client";

type Mode = "signup" | "login";

const copy = {
  signup: {
    title: "Create your account",
    subtitle: "Email and a password of at least 8 characters.",
    submit: "Create account",
    busy: "Creating account…",
    swapPrompt: "Already have an account?",
    swapLabel: "Log in",
    swapHref: "/login",
    passwordHint: "At least 8 characters.",
    autoComplete: "new-password",
  },
  login: {
    title: "Log in",
    subtitle: "Welcome back.",
    submit: "Log in",
    busy: "Logging in…",
    swapPrompt: "No account yet?",
    swapLabel: "Sign up",
    swapHref: "/signup",
    passwordHint: null,
    autoComplete: "current-password",
  },
} as const;

export function AuthForm({ mode, next }: { mode: Mode; next: string | null }) {
  const t = copy[mode];
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const target = safeNext(next);
  const swapHref = target
    ? `${t.swapHref}?next=${encodeURIComponent(target)}`
    : t.swapHref;

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      await postJson(`/api/auth/${mode}`, {
        email,
        password,
        ...(mode === "signup" && name.trim() ? { name: name.trim() } : {}),
      });
      // The session cookie decides where to land, so ask the server rather
      // than guessing from the form we just submitted.
      const me = await fetchMe();
      router.replace(destinationAfterAuth(me, target));
    } catch (err) {
      setError(errorMessage(err));
      setBusy(false);
    }
  }

  return (
    <div className="panel p-6 sm:p-8">
      <h1 className="text-xl font-semibold text-bone">{t.title}</h1>
      <p className="mt-1 text-sm text-slate">{t.subtitle}</p>

      <form className="mt-6 flex flex-col gap-4" onSubmit={onSubmit} noValidate>
        {mode === "signup" && (
          <div className="flex flex-col gap-1.5">
            <label htmlFor="name" className="text-sm text-bone-dim">
              Name <span className="text-slate">(optional)</span>
            </label>
            <input
              id="name"
              name="name"
              type="text"
              autoComplete="name"
              className="field"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={busy}
            />
          </div>
        )}

        <div className="flex flex-col gap-1.5">
          <label htmlFor="email" className="text-sm text-bone-dim">
            Email
          </label>
          <input
            id="email"
            name="email"
            type="email"
            required
            autoComplete="email"
            autoFocus
            className="field"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            disabled={busy}
          />
        </div>

        <div className="flex flex-col gap-1.5">
          <label htmlFor="password" className="text-sm text-bone-dim">
            Password
          </label>
          <input
            id="password"
            name="password"
            type="password"
            required
            minLength={mode === "signup" ? 8 : undefined}
            autoComplete={t.autoComplete}
            aria-describedby={t.passwordHint ? "password-hint" : undefined}
            className="field"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            disabled={busy}
          />
          {t.passwordHint && (
            <p id="password-hint" className="text-xs text-slate">
              {t.passwordHint}
            </p>
          )}
        </div>

        {error && <Notice tone="error">{error}</Notice>}

        <button type="submit" className="btn btn-primary mt-1" disabled={busy}>
          {busy ? t.busy : t.submit}
        </button>
      </form>

      <p className="mt-6 text-sm text-slate">
        {t.swapPrompt}{" "}
        <Link href={swapHref} className="link">
          {t.swapLabel}
        </Link>
      </p>
    </div>
  );
}
