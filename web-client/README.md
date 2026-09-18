# caveira web client

The caveira website and the backend both clients share, built with Next.js (App Router), TypeScript, Tailwind CSS, and Drizzle over libsql.

## Develop

```sh
bun install
bun run db:migrate
bun run dev
```

Then open http://localhost:3000. With no `.env` at all, the database is a SQLite file at `local.db` and billing runs in dev mode, where choosing a plan activates it without payment. Copy `.env.example` to `.env` to use Turso or Stripe; every variable is explained there.

## Build

```sh
bun run build
bun run start
```

## Database

The schema is in `src/db/schema.ts` and migrations live in `drizzle/`.

- `bun run db:generate` writes a new migration after you change the schema.
- `bun run db:migrate` applies migrations to `local.db`, or to Turso when `TURSO_DATABASE_URL` is set.
- `bun run db:studio` opens Drizzle Studio.

## Pages

- `/` is the landing page.
- `/signup` and `/login` take an email and password. Both accept `?next=` to return somewhere afterwards.
- `/pricing` shows the plan cards and starts checkout.
- `/cli` is where the terminal client sends you to approve its login code.
- `/account` shows your plan and links to Stripe's billing portal.

## API

Every route reads the session from the `caveira_session` cookie or from an `Authorization: Bearer` header, so the website and the CLI go through the same checks.

| Route | Purpose |
|---|---|
| `POST /api/auth/signup`, `POST /api/auth/login`, `POST /api/auth/logout` | Email and password accounts with cookie sessions |
| `GET /api/me` | The signed-in user, their subscription, and `hasAccess` |
| `POST /api/auth/device/start` | Issues a login code for the CLI |
| `POST /api/auth/device/approve` | A signed-in browser approves a code |
| `POST /api/auth/device/poll` | The CLI collects its token once the code is approved |
| `GET /api/plans` | The plan catalogue from `src/lib/plans.ts`, plus the billing mode |
| `POST /api/billing/checkout` | Starts Stripe Checkout, or activates the plan directly in dev billing mode |
| `POST /api/billing/portal` | Opens Stripe's customer portal |
| `POST /api/stripe/webhook` | Stripe subscription events, verified by signature, written to `subscriptions` |

`past_due` subscriptions still have access. Stripe retries a failed card for days, and the gate shouldn't lock someone out over an expired card while that happens.
