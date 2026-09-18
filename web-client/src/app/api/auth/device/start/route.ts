import { NextResponse } from "next/server";

import { db } from "@/db";
import { deviceCodes } from "@/db/schema";
import { newToken, newUserCode } from "@/lib/auth";
import { appUrl } from "@/lib/stripe";

const DEVICE_CODE_TTL_SECONDS = 15 * 60;

// Called by the CLI with no credentials at all: it is asking for a code, not
// claiming an identity. The pairing only becomes an identity once a signed-in
// browser approves it.
export async function POST() {
  const deviceCode = newToken();
  const userCode = newUserCode();
  const expiresAt = Math.floor(Date.now() / 1000) + DEVICE_CODE_TTL_SECONDS;

  await db.insert(deviceCodes).values({ deviceCode, userCode, expiresAt });

  return NextResponse.json({
    deviceCode,
    userCode,
    verificationUrl: `${appUrl()}/cli`,
    // Prefilled, so "open this link" skips the typing entirely; the short code
    // is the fallback for a machine with no browser of its own.
    verificationUrlComplete: `${appUrl()}/cli?code=${encodeURIComponent(userCode)}`,
    expiresIn: DEVICE_CODE_TTL_SECONDS,
    interval: 2,
  });
}
