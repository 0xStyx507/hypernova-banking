/**
 * Keeps the provider/domain context while hiding most of the local part.
 * Only the first and last two characters before `@` remain visible.
 */
export function maskEmail(value: string): string {
  const [localPart, domain] = value.trim().split("@", 2);
  if (!localPart || !domain) return "correo protegido";
  if (localPart.length <= 4) {
    const first = localPart.slice(0, 1);
    const last = localPart.slice(-1);
    return `${first}•••${last}@${domain}`;
  }
  return `${localPart.slice(0, 2)}•••${localPart.slice(-2)}@${domain}`;
}
