// theme.js — light/dark theme switching.
//
// All visual tokens live in styles/theme.css; toggling
// `document.documentElement.dataset.theme` swaps the active
// :root variable block. We persist the choice in localStorage
// so a refresh keeps the user's selection.
//
// On first load (no stored preference) we fall back to
// prefers-color-scheme so a system-set dark/light theme is
// respected without forcing the user to pick.

const STORAGE_KEY = 'theme';
export const THEMES = Object.freeze({ DARK: 'dark', LIGHT: 'light' });

function readStored() {
  try {
    const v = window.localStorage.getItem(STORAGE_KEY);
    return v === THEMES.DARK || v === THEMES.LIGHT ? v : null;
  } catch {
    return null;
  }
}

function writeStored(theme) {
  try {
    window.localStorage.setItem(STORAGE_KEY, theme);
  } catch {
    /* private browsing / quota — fail silently, the theme
       still applies for the current session. */
  }
}

function preferredFromSystem() {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return THEMES.DARK;
  }
  return window.matchMedia('(prefers-color-scheme: light)').matches
    ? THEMES.LIGHT
    : THEMES.DARK;
}

// Returns the theme that *should* apply right now. Stored choice
// wins; fall back to the OS preference; last-resort default is
// dark (matches the app's pre-theming look).
export function resolveInitialTheme() {
  return readStored() ?? preferredFromSystem();
}

// Apply a theme: writes the data-theme attribute on <html> and
// persists. Component CSS picks up the change automatically via
// the [data-theme="…"] selector blocks in theme.css.
export function setTheme(theme) {
  const next = theme === THEMES.LIGHT ? THEMES.LIGHT : THEMES.DARK;
  if (typeof document !== 'undefined') {
    document.documentElement.dataset.theme = next;
  }
  writeStored(next);
  return next;
}

export function getTheme() {
  if (typeof document === 'undefined') return THEMES.DARK;
  return document.documentElement.dataset.theme === THEMES.LIGHT
    ? THEMES.LIGHT
    : THEMES.DARK;
}

export function toggleTheme() {
  const next = getTheme() === THEMES.DARK ? THEMES.LIGHT : THEMES.DARK;
  return setTheme(next);
}

// Run once at app boot. Called from index.js BEFORE React mounts
// so the page paints in the correct theme from frame zero (avoids
// a dark→light flash for users who prefer light).
export function bootstrapTheme() {
  return setTheme(resolveInitialTheme());
}
