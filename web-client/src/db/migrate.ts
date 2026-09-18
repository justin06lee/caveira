import { createClient } from "@libsql/client";
import { drizzle } from "drizzle-orm/libsql";
import { migrate } from "drizzle-orm/libsql/migrator";

// Run with `bun run db:migrate`. Applies everything in drizzle/ to whichever
// database the environment points at — the local file by default, Turso when
// TURSO_DATABASE_URL is set.
const url = process.env.TURSO_DATABASE_URL ?? "file:./local.db";
const db = drizzle(createClient({ url, authToken: process.env.TURSO_AUTH_TOKEN }));

await migrate(db, { migrationsFolder: "./drizzle" });
console.log(`migrated ${url}`);
