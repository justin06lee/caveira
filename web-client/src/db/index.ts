import { createClient } from "@libsql/client";
import { drizzle } from "drizzle-orm/libsql";

import * as schema from "./schema";

// Turso in production, a plain SQLite file in development. Same driver either
// way, so nothing downstream has to care which one it is talking to.
const url = process.env.TURSO_DATABASE_URL ?? "file:./local.db";

export const db = drizzle(
  createClient({ url, authToken: process.env.TURSO_AUTH_TOKEN }),
  { schema },
);

export { schema };
