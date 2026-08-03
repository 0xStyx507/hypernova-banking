/** Keeps currency input friendly while sending integer minor units to the API. */
export function sanitizeCurrencyInput(value: string): string {
  const raw = value.replace(/[^\d.,]/g, "");
  if (!raw) return "";
  const lastDot = raw.lastIndexOf(".");
  const lastComma = raw.lastIndexOf(",");
  const decimalIndex = lastDot >= 0 && lastComma >= 0 ? Math.max(lastDot, lastComma) : (lastDot >= 0 ? lastDot : (lastComma >= 0 && raw.length - lastComma - 1 <= 2 ? lastComma : -1));
  const whole = (decimalIndex >= 0 ? raw.slice(0, decimalIndex) : raw).replace(/[.,]/g, "").replace(/^0+(?=\d)/, "");
  const fraction = decimalIndex >= 0 ? raw.slice(decimalIndex + 1).replace(/[^\d]/g, "").slice(0, 2) : "";
  return `${whole || "0"}${decimalIndex >= 0 ? `.${fraction}` : ""}`;
}

export function currencyInputToMinor(value: string): string {
  const normalized = sanitizeCurrencyInput(value);
  if (!/^\d+(?:\.\d{0,2})?$/u.test(normalized)) return "";
  const [whole, fraction = ""] = normalized.split(".");
  try { return (BigInt(whole) * 100n + BigInt(fraction.padEnd(2, "0") || "0")).toString(); } catch { return ""; }
}
