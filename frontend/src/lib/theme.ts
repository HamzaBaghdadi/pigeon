export type ThemeChoice = "system" | "light" | "dark";
export type StyleChoice = "default" | "blossom" | "gilded";

export const THEME_STORAGE_KEY = "pigeon.theme";
export const STYLE_STORAGE_KEY = "pigeon.style";

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

/** Applies a visual style (data-style on <html>); "default" removes it. */
export function applyStyle(style: StyleChoice) {
  const root = document.documentElement;
  if (style === "default") delete root.dataset.style;
  else root.dataset.style = style;
  try {
    localStorage.setItem(STYLE_STORAGE_KEY, style);
  } catch {
    /* cache only */
  }
}

export function toStyle(v: string | undefined): StyleChoice {
  return v === "blossom" || v === "gilded" ? v : "default";
}
