export type ThemeMode = "system" | "light" | "dark";

export const THEME_STORAGE_KEY = "hypernova.theme";

export function readThemeMode(): ThemeMode {
  // The approved Hypernova dashboard mockup is dark-first. Light remains
  // supported through the existing theme contract and device change listener.
  if (typeof window !== "undefined") {
    const stored = window.localStorage.getItem(THEME_STORAGE_KEY);
    if (stored === "light" || stored === "dark") return stored;
  }
  return "dark";
}

export function resolveTheme(mode: ThemeMode): ThemeMode {
  if (mode !== "system") return mode;
  if (typeof window === "undefined" || !window.matchMedia) return "light";
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}
