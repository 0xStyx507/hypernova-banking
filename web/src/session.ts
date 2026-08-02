import { Session } from "./types";

const SESSION_KEY = "hyperbank.web.session";

/**
 * Keeps the short-lived web session across a page reload without extending it
 * beyond the browser tab. The refresh token is still rotated by the API on
 * every restore, so a stale token is removed instead of being reused.
 */
export function readStoredSession(): Session | null {
  if (typeof window === "undefined") return null;
  try {
    const value = window.sessionStorage.getItem(SESSION_KEY);
    if (!value) return null;
    const parsed = JSON.parse(value) as Partial<Session>;
    if (!parsed.accessToken || !parsed.refreshToken || !parsed.user) return null;
    return parsed as Session;
  } catch {
    return null;
  }
}

export function storeSession(session: Session): void {
  if (typeof window === "undefined") return;
  window.sessionStorage.setItem(SESSION_KEY, JSON.stringify(session));
}

export function clearStoredSession(): void {
  if (typeof window === "undefined") return;
  window.sessionStorage.removeItem(SESSION_KEY);
}
