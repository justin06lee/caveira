import Link from "next/link";

import { AuthForm } from "@/components/auth-form";
import { CenteredMain, SiteFooter, SiteHeader } from "@/components/chrome";

export const metadata = { title: "Log in · caveira" };

export default async function LoginPage({
  searchParams,
}: {
  searchParams: Promise<{ [key: string]: string | string[] | undefined }>;
}) {
  const { next } = await searchParams;
  return (
    <>
      <SiteHeader
        right={
          <Link href="/pricing" className="text-slate transition-colors hover:text-bone">
            Pricing
          </Link>
        }
      />
      <CenteredMain>
        <AuthForm mode="login" next={typeof next === "string" ? next : null} />
      </CenteredMain>
      <SiteFooter />
    </>
  );
}
