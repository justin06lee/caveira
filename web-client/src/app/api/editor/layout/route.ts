import { readFile, writeFile } from "node:fs/promises";
import path from "node:path";

import { DEFAULT_LAYOUT } from "@/lib/default-layout";
import type { SiteLayout } from "@/lib/site-blocks";

// The editor is a local design tool, not a feature of the product: it writes a
// file into the working tree, so it exists only while `next dev` is running.
// In a production build both verbs 404 and nothing can touch the filesystem.
const DEV_ONLY = process.env.NODE_ENV !== "production";

const LAYOUT_PATH = path.join(process.cwd(), "site-layout.json");

export const dynamic = "force-dynamic";

export async function GET() {
  if (!DEV_ONLY) return new Response("Not found", { status: 404 });

  try {
    const raw = await readFile(LAYOUT_PATH, "utf8");
    return Response.json(JSON.parse(raw) as SiteLayout);
  } catch {
    // No file yet (or an unreadable one): hand back the shipped page so the
    // editor always opens on something real.
    return Response.json(DEFAULT_LAYOUT);
  }
}

export async function POST(request: Request) {
  if (!DEV_ONLY) return new Response("Not found", { status: 404 });

  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return Response.json({ error: "Body must be JSON." }, { status: 400 });
  }

  if (
    !body ||
    typeof body !== "object" ||
    !Array.isArray((body as { blocks?: unknown }).blocks)
  ) {
    return Response.json({ error: "Expected { blocks: [...] }." }, { status: 400 });
  }

  const layout: SiteLayout = {
    version: 1,
    updatedAt: new Date().toISOString(),
    blocks: (body as SiteLayout).blocks,
  };

  await writeFile(LAYOUT_PATH, `${JSON.stringify(layout, null, 2)}\n`, "utf8");
  return Response.json({ ok: true, savedAt: layout.updatedAt });
}
