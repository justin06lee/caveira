import { PricingBoard } from "@/components/pricing-board";

export const metadata = { title: "Pricing · caveira" };

export default async function PricingPage({
  searchParams,
}: {
  searchParams: Promise<{ [key: string]: string | string[] | undefined }>;
}) {
  const { checkout } = await searchParams;
  return <PricingBoard checkout={typeof checkout === "string" ? checkout : null} />;
}
