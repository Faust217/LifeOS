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

export function localDateTimeValue(now = new Date()): string {
  const local = new Date(now.getTime() - now.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

export function toOccurredAt(localValue: string): string {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(localValue)) throw new Error("Choose a valid date and time.");
  const date = new Date(localValue);
  if (Number.isNaN(date.getTime()) || localDateTimeValue(date) !== localValue) throw new Error("Choose a valid date and time.");
  if (date.getTime() > Date.now() + 60_000) throw new Error("The date and time cannot be in the future.");
  return date.toISOString();
}
