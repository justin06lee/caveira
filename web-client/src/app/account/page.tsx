import { AccountView } from "@/components/account-view";

export const metadata = { title: "Account · caveira" };

export default async function AccountPage({
  searchParams,
}: {
  searchParams: Promise<{ [key: string]: string | string[] | undefined }>;
}) {
  const { checkout } = await searchParams;
  return <AccountView checkout={typeof checkout === "string" ? checkout : null} />;
}
