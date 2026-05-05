import React, { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { FiSliders, FiChevronDown } from 'react-icons/fi';
import {
  getNotificationPreferences,
  updateNotificationPreferences,
} from '../services/api';
import {
  isPushSupported,
  getPushState,
  enablePush,
  disablePush,
} from '../utils/webPush';
import '../styles/NotificationSettings.css';

/*
 * NotificationSettings — 5 category toggles rendered behind a dropdown
 * trigger on the viewer's own profile page. The trigger sits inline
 * beside the privacy controls; the menu portals to document.body so
 * it escapes the profile hero's stacking context (`.profile-hero-main`
 * has z-index:1) and its `overflow: hidden` clip.
 *
 * Categories map to backend kinds via migration 017 / activity handler's
 * kindCategory map. The copy below is the only place that names user-
 * visible kinds — keep it in sync with the proto.
 */

const CATEGORY_META = [
  {
    key: 'mention',
    title: '@mentions',
    description: 'When someone tags you in a comment.',
  },
  {
    key: 'engagement',
    title: 'Engagement on your content',
    description: 'Likes, comments, and votes on your matchups & brackets.',
  },
  {
    key: 'milestone',
    title: 'Milestones',
    description: 'Round-number thresholds (100 votes, 1k followers, etc.)',
  },
  {
    key: 'prompt',
    title: 'Prompts & reminders',
    description: 'Matchup closing soon, tied results that need a winner.',
  },
  {
    key: 'social',
    title: 'Activity & follows',
    description: 'Your votes, bracket progress, outcomes, new followers.',
  },
  {
    key: 'email_digest',
    title: 'Weekly email digest',
    description: 'A Sunday recap of new activity, mentions, and picks from creators you follow.',
  },
];

const DEFAULT_PREFS = {
  mention: true,
  engagement: true,
  milestone: true,
  prompt: true,
  social: true,
  email_digest: true,
};

const MENU_WIDTH = 360;   // keep in sync with CSS max-width
const MENU_GAP = 8;       // px between trigger and menu
const VIEWPORT_PADDING = 12;

// Compute the menu's fixed-position rect from the trigger's bounding
// box. Prefers below the trigger, right-aligned to it; flips upward
// if there isn't room below. Clamps to viewport so the menu is always
// fully visible on narrow screens.
function computeMenuRect(triggerRect, menuHeight = 360) {
  if (!triggerRect) return null;
  const viewportW = window.innerWidth;
  const viewportH = window.innerHeight;

  const wantedWidth = Math.min(MENU_WIDTH, viewportW - VIEWPORT_PADDING * 2);
  let left = triggerRect.right - wantedWidth;
  if (left < VIEWPORT_PADDING) left = VIEWPORT_PADDING;
  if (left + wantedWidth > viewportW - VIEWPORT_PADDING) {
    left = viewportW - VIEWPORT_PADDING - wantedWidth;
  }

  const spaceBelow = viewportH - triggerRect.bottom - VIEWPORT_PADDING;
  const spaceAbove = triggerRect.top - VIEWPORT_PADDING;
  const openUpward = spaceBelow < menuHeight && spaceAbove > spaceBelow;

  let top;
  if (openUpward) {
    top = Math.max(VIEWPORT_PADDING, triggerRect.top - menuHeight - MENU_GAP);
  } else {
    top = triggerRect.bottom + MENU_GAP;
  }

  return { top, left, width: wantedWidth };
}

const NotificationSettings = () => {
  const [open, setOpen] = useState(false);
  const [prefs, setPrefs] = useState(DEFAULT_PREFS);
  const [loaded, setLoaded] = useState(false);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(null); // null | key currently saving
  const [error, setError] = useState(null);
  const [menuRect, setMenuRect] = useState(null);
  // Web Push state — 'unsupported' | 'denied' | 'default' | 'granted'
  // | 'pending' (we're mid-enable/disable flow). Loaded when the panel
  // opens; see the web-push helper for state definitions.
  const [pushState, setPushState] = useState('default');
  const [pushError, setPushError] = useState(null);
  const triggerRef = useRef(null);
  const menuRef = useRef(null);

  // Lazy load — the RPC doesn't fire until the user actually opens the
  // panel. Most profile visits never touch this UI, so deferring the
  // network hit keeps the page clean on first paint.
  useEffect(() => {
    if (!open || loaded) return undefined;
    let cancelled = false;
    setLoading(true);
    (async () => {
      try {
        const [prefsRes, pushS] = await Promise.all([
          getNotificationPreferences(),
          getPushState(),
        ]);
        if (cancelled) return;
        const next = prefsRes?.data?.prefs || {};
        // Merge onto defaults so any category missing from the server
        // response stays ON (mirrors the server-side fallback).
        setPrefs({ ...DEFAULT_PREFS, ...next });
        setPushState(pushS);
        setLoaded(true);
      } catch (err) {
        console.warn('NotificationSettings load failed', err);
        if (!cancelled) setError('Could not load preferences.');
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, loaded]);

  // Position the menu whenever it opens OR the viewport changes while
  // open. useLayoutEffect so the first paint already has coordinates
  // — otherwise the menu briefly flashes at 0,0 before settling.
  useLayoutEffect(() => {
    if (!open) return undefined;
    const reposition = () => {
      const triggerRect = triggerRef.current?.getBoundingClientRect();
      const menuH = menuRef.current?.offsetHeight;
      setMenuRect(computeMenuRect(triggerRect, menuH));
    };
    reposition();

    window.addEventListener('resize', reposition);
    // Capture-phase scroll listener catches scrolls on any ancestor
    // (profile page has its own scroll container in some layouts).
    window.addEventListener('scroll', reposition, true);
    return () => {
      window.removeEventListener('resize', reposition);
      window.removeEventListener('scroll', reposition, true);
    };
  }, [open, loaded, loading]);

  // Outside click + Escape close the dropdown. The menu is portaled so
  // we have to check both refs — contains() on the trigger alone would
  // miss clicks inside the portaled panel and close spuriously.
  useEffect(() => {
    if (!open) return undefined;
    const onClickOutside = (e) => {
      const clickedTrigger = triggerRef.current?.contains(e.target);
      const clickedMenu = menuRef.current?.contains(e.target);
      if (!clickedTrigger && !clickedMenu) {
        setOpen(false);
      }
    };
    const onKeyDown = (e) => {
      if (e.key === 'Escape') setOpen(false);
    };
    document.addEventListener('mousedown', onClickOutside);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('mousedown', onClickOutside);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [open]);

  const handlePushClick = useCallback(async () => {
    setPushError(null);
    const previous = pushState;
    setPushState('pending');
    try {
      if (previous === 'granted') {
        await disablePush();
        setPushState('default');
      } else {
        await enablePush();
        setPushState('granted');
      }
    } catch (err) {
      console.warn('webpush toggle failed', err);
      setPushError(err?.message || 'Could not update push state.');
      // Restore the previous state so the button text doesn't lie.
      setPushState(previous);
    }
  }, [pushState]);

  const handleToggle = useCallback(
    async (key) => {
      const next = { ...prefs, [key]: !prefs[key] };
      // Optimistic update — revert on failure.
      setPrefs(next);
      setSaving(key);
      setError(null);
      try {
        await updateNotificationPreferences(next);
      } catch (err) {
        console.warn('NotificationSettings save failed', err);
        setPrefs(prefs);
        setError('Could not save. Try again.');
      } finally {
        setSaving((curr) => (curr === key ? null : curr));
      }
    },
    [prefs]
  );

  // A summary chip when something is muted — quick visual cue that the
  // feed is filtered without opening the panel.
  const mutedCount = loaded
    ? CATEGORY_META.filter(({ key }) => prefs[key] === false).length
    : 0;

  const menu = open ? (
    <div
      ref={menuRef}
      className="notification-settings__menu"
      role="dialog"
      aria-label="Notification preferences"
      style={
        menuRect
          ? { top: menuRect.top, left: menuRect.left, width: menuRect.width }
          : { visibility: 'hidden' }
      }
    >
      <header className="notification-settings__menu-header">
        <h3>Notifications</h3>
        <p>Choose which categories show up in your feed.</p>
      </header>

      {loading && !loaded ? (
        <div className="notification-settings__state">Loading…</div>
      ) : (
        <ul className="notification-settings__list">
          {CATEGORY_META.map(({ key, title, description }) => {
            const enabled = prefs[key] !== false;
            return (
              <li className="notification-settings__item" key={key}>
                <div className="notification-settings__copy">
                  <span className="notification-settings__title">{title}</span>
                  <span className="notification-settings__description">{description}</span>
                </div>
                <label className="notification-settings__toggle">
                  <input
                    type="checkbox"
                    checked={enabled}
                    onChange={() => handleToggle(key)}
                    disabled={saving === key}
                    aria-label={`Toggle ${title}`}
                  />
                  <span className="notification-settings__slider" aria-hidden="true" />
                </label>
              </li>
            );
          })}
        </ul>
      )}

      {/* Web Push opt-in — separate from the category toggles because
          it has a distinct permission flow (browser-level consent) and
          runs on a separate delivery channel. Hidden entirely on
          browsers without Web Push support. */}
      {isPushSupported() && (
        <div className="notification-settings__push">
          <div className="notification-settings__copy">
            <span className="notification-settings__title">Browser push notifications</span>
            <span className="notification-settings__description">
              {pushState === 'denied'
                ? 'Push is blocked for this site. Allow it in your browser settings to turn on push here.'
                : 'Get a native notification for mentions, milestones, closing-soon, and ties.'}
            </span>
          </div>
          <button
            type="button"
            className="notification-settings__push-button"
            onClick={handlePushClick}
            disabled={pushState === 'pending' || pushState === 'denied'}
          >
            {pushState === 'granted' && 'Disable'}
            {pushState === 'pending' && '…'}
            {pushState === 'default' && 'Enable'}
            {pushState === 'denied' && 'Blocked'}
          </button>
        </div>
      )}

      {error && <div className="notification-settings__error">{error}</div>}
      {pushError && <div className="notification-settings__error">{pushError}</div>}
    </div>
  ) : null;

  return (
    <div className="notification-settings">
      <button
        ref={triggerRef}
        type="button"
        className={`notification-settings__trigger ${open ? 'is-open' : ''}`}
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="true"
        aria-expanded={open}
      >
        <FiSliders aria-hidden="true" />
        <span>Notifications</span>
        {mutedCount > 0 && (
          <span className="notification-settings__muted-badge">
            {mutedCount} muted
          </span>
        )}
        <FiChevronDown
          aria-hidden="true"
          className={`notification-settings__chevron ${open ? 'is-open' : ''}`}
        />
      </button>

      {menu && typeof document !== 'undefined'
        ? createPortal(menu, document.body)
        : menu}
    </div>
  );
};

export default NotificationSettings;
