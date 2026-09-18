import { DeviceApproval } from "@/components/device-approval";

export const metadata = { title: "Connect your terminal · caveira" };

export default async function CliPage({
  searchParams,
}: {
  searchParams: Promise<{ [key: string]: string | string[] | undefined }>;
}) {
  const { code } = await searchParams;
  return <DeviceApproval code={typeof code === "string" ? code : null} />;
}
