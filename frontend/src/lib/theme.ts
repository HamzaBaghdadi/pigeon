export type ThemeChoice = "system" | "light" | "dark";

export const THEME_STORAGE_KEY = "pigeon.theme";

/**
 * Applies a theme to the document. "system" removes the override so the
 * prefers-color-scheme media query (which follows Windows) takes over.
 */
export function applyTheme(theme: ThemeChoice) {
  const root = document.documentElement;
  if (theme === "system") delete root.dataset.theme;
  else root.dataset.theme = theme;
  try {
    localStorage.setItem(THEME_STORAGE_KEY, theme);
  } catch {
    /* cache only; Go settings remain the source of truth */
  }
}
