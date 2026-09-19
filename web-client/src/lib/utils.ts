// Tiny className joiner. No clsx/tailwind-merge dependency: we never pass
// conflicting utilities to the same element, so a plain filtered join is
// enough and keeps the bundle honest.
export function cn(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(" ");
}
