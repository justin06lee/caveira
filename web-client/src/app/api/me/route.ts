import { NextResponse, type NextRequest } from "next/server";

import { getViewer, publicUser } from "@/lib/auth";

export async function GET(req: NextRequest) {
  const viewer = await getViewer(req);
  if (!viewer) return NextResponse.json({ error: "Not signed in." }, { status: 401 });
  return NextResponse.json(publicUser(viewer));
}
