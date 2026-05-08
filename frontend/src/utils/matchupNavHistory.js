// Per-tab navigation history for the matchup-detail swipe stream.
//
// MatchupPage uses this to power "Next matchup →" + "← Previous"
// buttons à la Twitter video swipe. The stack lives in
// sessionStorage so a tab refresh keeps your place; a new tab
// (or a fresh browser session) starts clean.
//
// State shape:
//   { history: Array<{ id: string, slug: string }>, cursor: number }
//
// All I/O is wrapped in try/catch so private-browsing or quota-
// exceeded errors degrade silently — handlers fall back to the
// in-memory copy for the current session.

const KEY = 'matchup-nav-history';

const EMPTY = Object.freeze({ history: [], cursor: 0 });

export function readHistory() {
  try {
    const raw = sessionStorage.getItem(KEY);
    if (!raw) return { history: [], cursor: 0 };
    const parsed = JSON.parse(raw);
    if (!parsed || !Array.isArray(parsed.history) || typeof parsed.cursor !== 'number') {
      return { history: [], cursor: 0 };
    }
    // Defensive bounds: cursor outside the array shouldn't be
    // possible, but a stale entry from an older schema should
    // self-heal rather than crash the page.
    if (parsed.cursor < 0 || parsed.cursor >= parsed.history.length) {
      return { history: parsed.history, cursor: 0 };
    }
    return parsed;
  } catch {
    return { history: [], cursor: 0 };
  }
}

export function writeHistory(state) {
  try {
    sessionStorage.setItem(KEY, JSON.stringify(state));
  } catch {
    // sessionStorage unavailable — silently degrade.
  }
}

export const _emptyHistory = EMPTY;
