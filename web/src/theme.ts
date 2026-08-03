export type ThemeMode = "light";

export const THEME_STORAGE_KEY = "hypernova.theme";

export function readThemeMode(): ThemeMode {
  return "light";
}

export function resolveTheme(_mode: ThemeMode): "light" {
  return "light";
}
