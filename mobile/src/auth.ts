/**
 * Presentation helpers for authentication screens.
 *
 * These helpers never change the value sent to the API. They only protect
 * sensitive identity data while it is rendered to the user.
 */
export function maskEmail(email: string): string {
  const normalized = email.trim();
  const separator = normalized.indexOf("@");
  if (separator <= 0 || separator === normalized.length - 1) return "tu cuenta";

  const localPart = normalized.slice(0, separator);
  const domain = normalized.slice(separator + 1);
  if (localPart.length <= 4) {
    return `${localPart.slice(0, 1)}•••${localPart.slice(-1)}@${domain}`;
  }
  const firstCharacters = localPart.slice(0, 2);
  const lastCharacters = localPart.slice(-2);
  const hiddenLength = Math.max(3, localPart.length - firstCharacters.length - lastCharacters.length);

  return `${firstCharacters}${"•".repeat(hiddenLength)}${lastCharacters}@${domain}`;
}

/** Keeps MFA input numeric without duplicating server-side validation rules. */
export function sanitizeMfaCode(value: string): string {
  return value.replace(/\D/g, "").slice(0, 6);
}

/** Validates the email shape locally; mailbox ownership requires verification. */
export function isValidEmail(value: string): boolean {
  const normalized = value.trim().toLowerCase();
  return normalized.length <= 320 && /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/u.test(normalized);
}
