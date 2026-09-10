// Theme resolution (D8, light mode). Shared by Shell.tsx (the toggle) and,
// additively, by anything that needs to know which palette is currently
// showing (e.g. web/src/visual/schemaAdapter.ts picking a wire/category
// color) — see currentTheme() below.
//
// Contract: with no stored choice, prefers-color-scheme decides (index.css's
// `@media (prefers-color-scheme: light)` block, guarded by `:not(.dark)`,
// handles that half with no JS involved at all). The toggle writes an
// EXPLICIT choice to localStorage under STORAGE_KEY and sets `html.light` or
// `html.dark`, which both override the media query (see index.css). Only an
// explicit toggle click persists — resolving the initial theme from the
// system default must not silently turn "no preference set" into a stored
// preference (Shell.tsx applies this by only calling setStoredTheme after
// the first render; see its effect).

export type Theme = 'light' | 'dark';

const STORAGE_KEY = 'theme';

/** Reads the explicit stored choice, or null if unset/invalid/unavailable. */
export function getStoredTheme(): Theme | null {
  if (typeof window === 'undefined') return null;
  try {
    const v = window.localStorage.getItem(STORAGE_KEY);
    return v === 'light' || v === 'dark' ? v : null;
  } catch {
    // localStorage can throw (private browsing, quota, disabled) — treat as unset.
    return null;
  }
}

/** Persists an explicit theme choice. Call this ONLY from a user action (the
 * toggle) — never to record the system-derived default, or "no preference
 * set" silently becomes "light/dark was chosen" forever. */
export function setStoredTheme(theme: Theme): void {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(STORAGE_KEY, theme);
  } catch {
    /* ignore persistence failures; the toggle still works for this session */
  }
}

/** The OS/browser's preference, defaulting to 'dark' when unavailable (SSR,
 * a test environment with no matchMedia, or a browser that doesn't support
 * the media feature) — matches index.css's dark-by-default @theme block. */
export function getSystemTheme(): Theme {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return 'dark';
  return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
}

/** The theme to apply right now: the stored explicit choice if there is one,
 * otherwise the system preference. */
export function resolveTheme(): Theme {
  return getStoredTheme() ?? getSystemTheme();
}

/** Sets exactly one of `html.light` / `html.dark` (never both), matching the
 * selectors index.css's light-palette overrides key off. Idempotent. */
export function applyTheme(theme: Theme): void {
  if (typeof document === 'undefined') return;
  const root = document.documentElement;
  root.classList.toggle('dark', theme === 'dark');
  root.classList.toggle('light', theme === 'light');
}

/**
 * The theme CURRENTLY in effect, read back from the DOM rather than
 * recomputed — an explicit `html.light`/`html.dark` class (set by the
 * toggle, or by index.html's inline bootstrap script before React mounts)
 * wins; with neither class present, falls back to the system preference,
 * mirroring index.css's unguarded `@media (prefers-color-scheme)` default.
 *
 * Additive: not yet called from Shell.tsx or anywhere in web/src/visual —
 * it exists so schemaAdapter's getWireColor/getCategoryColor (and any future
 * caller) can pick a theme-appropriate color without each duplicating this
 * resolution logic. Wave 2 wires callers onto it.
 */
export function currentTheme(): Theme {
  if (typeof document === 'undefined') return getSystemTheme();
  const root = document.documentElement;
  if (root.classList.contains('light')) return 'light';
  if (root.classList.contains('dark')) return 'dark';
  return getSystemTheme();
}
