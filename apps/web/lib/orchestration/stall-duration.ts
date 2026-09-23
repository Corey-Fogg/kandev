const UNIT_SECONDS: Record<string, number> = {
  h: 3600,
  m: 60,
  s: 1,
  ms: 1e-3,
  us: 1e-6,
  µs: 1e-6,
  ns: 1e-9,
};
const TOKEN = /(\d+(?:\.\d+)?)(h|ms|m|s|us|µs|ns)/g;
const WHOLE = /^(?:\d+(?:\.\d+)?(?:h|ms|m|s|us|µs|ns))+$/;

/**
 * Seconds in a Go `time.Duration` string such as "1h2m3s" or "2h0m0.6s", as
 * the backend stores a stall's length. Anything else is null.
 */
export function parseGoDuration(value: string | undefined): number | null {
  if (!value || !WHOLE.test(value)) return null;
  let seconds = 0;
  for (const [, amount, unit] of value.matchAll(TOKEN))
    seconds += Number(amount) * UNIT_SECONDS[unit];
  return seconds;
}

/**
 * A stall length for display in the given locale: hours and minutes (or
 * minutes alone under an hour, and at least one minute), formatted by Intl so
 * no unit words are hard-coded.
 */
export function formatStallDuration(seconds: number, locale: string): string {
  const totalMinutes = Math.max(1, Math.floor(seconds / 60));
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  const unit = (value: number, name: "hour" | "minute") =>
    new Intl.NumberFormat(locale, { style: "unit", unit: name, unitDisplay: "long" }).format(value);
  const parts = [hours > 0 ? unit(hours, "hour") : "", minutes > 0 ? unit(minutes, "minute") : ""];
  return new Intl.ListFormat(locale, { style: "long", type: "unit" }).format(parts.filter(Boolean));
}
