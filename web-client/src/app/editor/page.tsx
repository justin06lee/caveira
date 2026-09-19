import { notFound } from "next/navigation";

import { EditorShell } from "@/components/editor/editor-shell";

export const metadata = { title: "Site editor · caveira" };

/**
 * A local design tool, not a page of the product: it writes `site-layout.json`
 * into the working tree, so it exists only under `next dev`. The API it talks
 * to refuses in production too — this is the front door, not the only lock.
 */
export default function EditorPage() {
  if (process.env.NODE_ENV === "production") notFound();
  return <EditorShell />;
}
