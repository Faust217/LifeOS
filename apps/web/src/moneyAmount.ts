const MAX_INT64 = 9223372036854775807n;
const MIN_INT64 = -9223372036854775808n;

export function currencyDigits(currency: string): number {
  if (!/^[A-Z]{3}$/.test(currency)) throw new Error("Enter a three-letter currency code.");
  try {
    return new Intl.NumberFormat("en", { style: "currency", currency }).resolvedOptions().maximumFractionDigits ?? 2;
  } catch {
    throw new Error("Enter a valid ISO currency code.");
  }
}

export function parseAmountToMinor(input: string, currency: string, allowNegative = false): string {
  const digits = currencyDigits(currency);
  const value = input.trim();
  const match = /^(-?)(0|[1-9][0-9]*)(?:\.([0-9]+))?$/.exec(value);
  if (!match || (match[1] && !allowNegative)) throw new Error("Enter a valid amount without separators or a currency symbol.");
  const fraction = match[3] ?? "";
  if (fraction.length > digits || (digits === 0 && fraction.length > 0)) {
    throw new Error(`Use no more than ${digits} decimal place${digits === 1 ? "" : "s"} for ${currency}.`);
  }
  const whole = BigInt(match[2]);
  const scale = 10n ** BigInt(digits);
  const minor = (whole * scale + BigInt(fraction.padEnd(digits, "0") || "0")) * (match[1] ? -1n : 1n);
  if (minor > MAX_INT64 || minor < MIN_INT64) throw new Error("The amount is outside the supported range.");
  if (!allowNegative && minor <= 0n) throw new Error("Enter an amount greater than zero.");
  return minor.toString();
}

export function formatMinorUnits(minorText: string, currency: string): string {
  const digits = currencyDigits(currency);
  const minor = BigInt(minorText);
  const negative = minor < 0n;
  const absolute = negative ? -minor : minor;
  const scale = 10n ** BigInt(digits);
  const whole = new Intl.NumberFormat("en-US").format(absolute / scale);
  const fraction = digits ? `.${(absolute % scale).toString().padStart(digits, "0")}` : "";
  return `${negative ? "−" : ""}${currency} ${whole}${fraction}`;
}

export function minorUnitsToInput(minorText: string, currency: string): string {
  const digits = currencyDigits(currency);
  const minor = BigInt(minorText);
  const absolute = minor < 0n ? -minor : minor;
  const scale = 10n ** BigInt(digits);
  return `${minor < 0n ? "-" : ""}${absolute / scale}${digits ? `.${(absolute % scale).toString().padStart(digits, "0")}` : ""}`;
}

function zonedParts(instant: Date, zone: string): Record<string, string> {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: zone, calendar: "gregory", numberingSystem: "latn", hourCycle: "h23",
    year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit",
  }).formatToParts(instant);
  return Object.fromEntries(parts.map(({ type, value }) => [type, value]));
}

export function localDateTimeValue(now: Date, zone: string): string {
  const p = zonedParts(now, zone);
  return `${p.year}-${p.month}-${p.day}T${p.hour}:${p.minute}`;
}

export function localDateKey(instant: string, zone: string): string {
  return localDateTimeValue(new Date(instant), zone).slice(0, 10);
}

export function formatLocalDate(instant: string, zone: string): string {
  return new Intl.DateTimeFormat("en", { timeZone: zone, dateStyle: "full" }).format(new Date(instant));
}

export function formatLocalTime(instant: string, zone: string): string {
  return new Intl.DateTimeFormat("en-GB", { timeZone: zone, hourCycle: "h23", hour: "2-digit", minute: "2-digit" }).format(new Date(instant));
}

export function formatLocalTimeWithZone(instant: string, zone: string): string {
  return new Intl.DateTimeFormat("en", { timeZone: zone, hour: "numeric", minute: "2-digit", timeZoneName: "short" }).format(new Date(instant));
}

export function timeZoneLabel(zone: string, now = new Date()): string {
  const place = zone.split("/").at(-1)?.replaceAll("_", " ") ?? zone;
  const offset = new Intl.DateTimeFormat("en", { timeZone: zone, timeZoneName: "shortOffset" })
    .formatToParts(now).find((part) => part.type === "timeZoneName")?.value ?? "GMT";
  return `${place} (${offset === "GMT" ? "UTC+0" : offset.replace(/^GMT/, "UTC")})`;
}

// A wall clock can map to zero instants at a DST gap or two at a DST overlap.
// Sample offsets around the date, then round-trip every candidate in the chosen zone.
export function matchingInstants(localValue: string, zone: string): string[] {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(localValue)) throw new Error("Choose a valid date and time.");
  const nominal = Date.parse(`${localValue}:00Z`);
  if (!Number.isFinite(nominal) || new Date(nominal).toISOString().slice(0, 16) !== localValue) throw new Error("Choose a valid date and time.");
  const offsets = new Set<number>();
  for (let hour = -36; hour <= 36; hour += 3) {
    const sample = nominal + hour * 3_600_000;
    const p = zonedParts(new Date(sample), zone);
    const wall = Date.UTC(Number(p.year), Number(p.month) - 1, Number(p.day), Number(p.hour), Number(p.minute), Number(p.second));
    offsets.add(wall - sample);
  }
  const matches = [...offsets].map((offset) => nominal - offset)
    .filter((candidate) => localDateTimeValue(new Date(candidate), zone) === localValue)
    .sort((a, b) => a - b);
  if (matches.length === 0) throw new Error("That time does not exist in this timezone. Choose another time.");
  return matches.map((candidate) => new Date(candidate).toISOString());
}

export function toOccurredAt(localValue: string, zone: string, chosenInstant?: string): string {
  const matches = matchingInstants(localValue, zone);
  if (matches.length > 1 && !chosenInstant) throw new Error("This time occurs twice here. Choose which occurrence you mean.");
  const instant = chosenInstant ?? matches[0];
  if (!matches.includes(instant)) throw new Error("Choose a valid occurrence of this local time.");
  if (Date.parse(instant) > Date.now() + 60_000) throw new Error("The date and time cannot be in the future.");
  return instant;
}
