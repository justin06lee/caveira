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

## Site editor

`/editor` is a local design tool for laying out the marketing site. It is **dev only**: under `next start` the page 404s and the API behind it refuses, because it writes a file into the working tree.

A layout is an ordered list of blocks — hero, ticker, statement, feature grid, split, chip row, steps, pricing, quotes, FAQ, closing CTA. Pick a block in the outline on the left, edit its fields on the right, and the middle pane renders it with the real site components, so the preview is the page. Changes autosave to `site-layout.json` about a second after you stop typing; *reset* restores the page as it currently ships.

- `src/lib/site-blocks.ts` is the vocabulary: block types, their defaults, and the field schema that generates the inspector. Adding a field there adds it to the UI.
- `src/lib/default-layout.ts` is the shipped landing page expressed as blocks — what the editor opens on when there is no file yet.
- `src/components/site/block-renderer.tsx` draws a layout. It has no server-only imports, so both the editor and a page can use it.
- `site-layout.json` is the saved layout. It is a design document, not something the site reads at runtime: `src/app/page.tsx` stays hand-written.

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
