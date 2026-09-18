// Browser-side API helper. Every page talks to the JSON API the same way:
// same-origin fetch, cookies included, errors surfaced as a thrown ApiError
// carrying the status and the server's own message so pages can render it
// inline instead of inventing their own wording.

export type Subscription = {
  planId: string;
  status: string;
  currentPeriodEnd: number | null;
  cancelAtPeriodEnd: boolean | null;
};

export type Me = {
  id: string;
  email: string;
  name: string | null;
  subscription: Subscription | null;
  hasAccess: boolean;
};

export type PlanCard = {
  id: string;
  name: string;
  tagline: string;
  priceUsd: number;
  interval: string;
  features: string[];
};

export type PlansResponse = {
  plans: PlanCard[];
  billing: "stripe" | "dev";
};

export type CheckoutResponse =
  | { mode: "stripe"; url: string }
  | { mode: "dev"; planId: string; activated: true };

export class ApiError extends Error {
  status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, {
      credentials: "same-origin",
      ...init,
      headers: {
        ...(init?.body ? { "content-type": "application/json" } : {}),
        ...init?.headers,
      },
    });
  } catch {
    throw new ApiError("Could not reach the server. Check your connection.", 0);
  }

  const text = await res.text();
  let payload: unknown = null;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = null;
    }
  }

  if (!res.ok) {
    const message =
      payload &&
      typeof payload === "object" &&
      typeof (payload as { error?: unknown }).error === "string"
        ? (payload as { error: string }).error
        : `Something went wrong (${res.status}).`;
    throw new ApiError(message, res.status);
  }

  return payload as T;
}

export function getJson<T>(path: string): Promise<T> {
  return request<T>(path, { method: "GET", cache: "no-store" });
}

export function postJson<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: "POST",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

/** `null` means "not signed in"; anything else is a real failure worth showing. */
export async function fetchMe(): Promise<Me | null> {
  try {
    return await getJson<Me>("/api/me");
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) return null;
    throw err;
  }
}

export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error && err.message) return err.message;
  return "Something went wrong.";
}

/**
 * Only same-site paths survive a `?next=` round trip: a full URL here would be
 * an open redirect, and `//host` is a URL wearing a path's clothes.
 */
export function safeNext(next: string | null | undefined): string | null {
  if (!next) return null;
  if (!next.startsWith("/") || next.startsWith("//")) return null;
  return next;
}

/** Where someone lands after signing in: their errand, else plan or account. */
export function destinationAfterAuth(
  me: Me | null,
  next: string | null | undefined,
): string {
  return safeNext(next) ?? (me?.hasAccess ? "/account" : "/pricing");
}

export function formatDate(unixSeconds: number | null | undefined): string {
  if (!unixSeconds) return "—";
  return new Date(unixSeconds * 1000).toLocaleDateString(undefined, {
    year: "numeric",
    month: "long",
    day: "numeric",
  });
}

export function formatStatus(status: string): string {
  return status.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}
