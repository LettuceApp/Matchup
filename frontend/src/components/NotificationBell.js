import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { FiBell } from 'react-icons/fi';
import ActivityFeed from './ActivityFeed';
import { getUserActivity, markActivityRead } from '../services/api';
import { groupActivityItems, unreadCountFromGrouped } from '../utils/activityGrouping';
import '../styles/NotificationBell.css';

/*
 * NotificationBell — auth-only header affordance that surfaces recent
 * activity without making users navigate to their profile. Renders
 * nothing for signed-out visitors.
 *
 * Behavior:
 *   - Mounts the moment the user is authed; pulls the last ~10
 *     activity items via `getUserActivity` and polls every 60s while
 *     visible.
 *   - Unread count = number of items with `occurred_at` newer than the
 *     `activity_last_seen_at:{me}` localStorage timestamp. Clicking
 *     the bell opens a panel AND stamps localStorage to "now", so the
 *     badge clears the moment the user acknowledges.
 *   - Panel reuses the <ActivityFeed> component so the visual grammar
 *     matches the profile Activity tab exactly. Footer has a "See all
 *     activity" link that deep-links to the profile page's Activity
 *     tab via `?tab=activity`.
 *   - Outside click / Escape closes the panel.
 *
 * This is deliberately polling rather than SSE. SSE push is a
 * documented follow-up in the plan file; poll-on-open is the minimum
 * viable shape and keeps the component dependency-free.
 */

const BELL_LIMIT = 10;          // items to fetch for the dropdown
const POLL_INTERVAL_MS = 60_000; // quiet refresh cadence

function storageKeyFor(userKey) {
  return `activity_last_seen_at:${userKey}`;
}

function readLastSeen(userKey) {
  try {
    return window.localStorage.getItem(storageKeyFor(userKey));
  } catch {
    return null;
  }
}

function writeLastSeenNow(userKey) {
  try {
    window.localStorage.setItem(storageKeyFor(userKey), new Date().toISOString());
  } catch {
    // localStorage unavailable (private mode etc.) — silently accept
    // that the badge will re-appear on next load. Not worth surfacing.
  }
}

const NotificationBell = () => {
  // Read auth + identifier from localStorage on each render. Cheap; no
  // need for context state for this tiny component, and matches how
  // NavigationBar reads it.
  const userId = typeof window !== 'undefined' ? localStorage.getItem('userId') : null;
  const username = typeof window !== 'undefined' ? localStorage.getItem('username') : null;
  const isAuthed = typeof window !== 'undefined' ? Boolean(localStorage.getItem('token')) : false;

  // Prefer username for the storage key + RPC lookup (matches the
  // profile-page convention), fall back to UUID.
  const userKey = username && username !== 'undefined' ? username : userId;

  const [open, setOpen] = useState(false);
  const [items, setItems] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [lastSeenAtAtOpen, setLastSeenAtAtOpen] = useState(null);
  const rootRef = useRef(null);

  // Fetch the feed. Called on mount, on open, and every POLL_INTERVAL_MS.
  const fetchActivity = useCallback(async () => {
    if (!userKey) return;
    setLoading(true);
    setError(null);
    try {
      const res = await getUserActivity(userKey, { limit: BELL_LIMIT });
      const next = Array.isArray(res.data?.items) ? res.data.items : [];
      setItems(next);
    } catch (err) {
      console.warn('NotificationBell fetch failed', err);
      setError('Activity unavailable.');
    } finally {
      setLoading(false);
    }
  }, [userKey]);

  // Initial load + poll. Re-subscribe when the user changes.
  useEffect(() => {
    if (!isAuthed || !userKey) return undefined;
    fetchActivity();
    const t = setInterval(fetchActivity, POLL_INTERVAL_MS);
    return () => clearInterval(t);
  }, [isAuthed, userKey, fetchActivity]);

  // SSE push — hitch a ride on the Redis pub/sub channel so new events
  // appear within a second instead of up to 60s via polling. The
  // payload is just `ping`; we always refetch, matching the derived-
  // feed read model. Reconnect on error with a 10s backoff so a Redis
  // blip doesn't leave the bell offline; the 60s poll above is a
  // perfectly fine fallback in the meantime.
  useEffect(() => {
    if (!isAuthed || !userKey) return undefined;
    if (typeof window === 'undefined' || typeof EventSource === 'undefined') {
      return undefined;
    }

    let es = null;
    let retryTimer = null;
    let cancelled = false;

    const connect = () => {
      if (cancelled) return;
      try {
        es = new EventSource('/users/me/activity/events', { withCredentials: true });
      } catch (err) {
        // Some browsers can throw synchronously; fall back to poll.
        console.warn('NotificationBell EventSource failed to open', err);
        return;
      }
      es.onmessage = () => {
        fetchActivity();
      };
      es.onerror = () => {
        if (es) {
          es.close();
          es = null;
        }
        if (cancelled) return;
        // Retry with a gentle delay. The poll loop stays active in
        // the meantime so the bell isn't blind.
        retryTimer = setTimeout(connect, 10_000);
      };
    };

    connect();

    return () => {
      cancelled = true;
      if (retryTimer) clearTimeout(retryTimer);
      if (es) es.close();
    };
  }, [isAuthed, userKey, fetchActivity]);

  // Outside-click + Escape close the dropdown. Only bound while open.
  //
  // pointerdown (not mousedown) because mobile-Safari sometimes
  // dispatches synthesized mousedown events with `e.target` set to
  // document.body during a tap — which made `contains(e.target)`
  // return false, closed the panel, and unmounted the Link the user
  // had just touched before the click event could land. Net effect:
  // tapping any notification on mobile did nothing. pointerdown
  // fires for both touch + mouse with a reliable target, fixing
  // the gesture-eats-the-link bug.
  useEffect(() => {
    if (!open) return undefined;
    const onClickOutside = (e) => {
      if (rootRef.current && !rootRef.current.contains(e.target)) {
        setOpen(false);
      }
    };
    const onKeyDown = (e) => {
      if (e.key === 'Escape') setOpen(false);
    };
    document.addEventListener('pointerdown', onClickOutside);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('pointerdown', onClickOutside);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [open]);

  // Collapse repeat (subject_id, kind) events into groups before we
  // count or render. Keeps the badge honest on viral matchups — 40
  // votes on one matchup = 1 unread entry, not 40. The Activity tab
  // doesn't use this helper; only the bell dropdown.
  const groupedItems = useMemo(() => groupActivityItems(items), [items]);

  // Unread count re-computed from grouped items + current stored
  // seen-at. useMemo keeps this stable across re-renders with the
  // same inputs (the badge otherwise twinkles on every render cycle).
  const unreadCount = useMemo(() => {
    if (!userKey) return 0;
    return unreadCountFromGrouped(groupedItems, readLastSeen(userKey));
    // Re-evaluate on open, too — the dropdown opening is what marks
    // as "read" and we want the stored seen-at to advance.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [groupedItems, userKey, open]);

  // Toggle handler — snapshots lastSeenAt for the dropdown render
  // BEFORE updating, so items arriving before this open still carry
  // the unread tint inside the panel. Also stamps persisted
  // notifications server-side so the cross-device badge state stays
  // in sync (localStorage alone would only clear on the device that
  // opened the bell).
  const toggleOpen = () => {
    setOpen((prev) => {
      const next = !prev;
      if (next) {
        const previousSeen = readLastSeen(userKey);
        setLastSeenAtAtOpen(previousSeen);
        writeLastSeenNow(userKey);
        // Fire-and-forget — server stamping is for the next refetch /
        // next device. Failure falls through to the lastSeenAt-only
        // behavior, which is what we already had pre-#6.
        markActivityRead().catch((err) => {
          console.warn('NotificationBell mark-read failed', err);
        });
      }
      return next;
    });
  };

  if (!isAuthed || !userKey) return null;

  const badgeLabel = unreadCount > 9 ? '9+' : String(unreadCount);

  return (
    <div className="notification-bell" ref={rootRef}>
      <button
        type="button"
        className={`notification-bell__button ${open ? 'is-open' : ''}`}
        onClick={toggleOpen}
        aria-label={unreadCount > 0 ? `Notifications, ${unreadCount} unread` : 'Notifications'}
        aria-haspopup="true"
        aria-expanded={open}
      >
        <FiBell aria-hidden="true" />
        {unreadCount > 0 && (
          <span className="notification-bell__badge" aria-hidden="true">
            {badgeLabel}
          </span>
        )}
      </button>

      {open && (
        <div className="notification-bell__panel" role="dialog" aria-label="Recent activity">
          <header className="notification-bell__panel-header">
            <span>Activity</span>
            {loading && <span className="notification-bell__loading">refreshing…</span>}
          </header>

          <div className="notification-bell__panel-body">
            <ActivityFeed
              items={groupedItems}
              lastSeenAt={lastSeenAtAtOpen}
              loading={loading && groupedItems.length === 0}
              error={error}
              onRetry={fetchActivity}
            />
          </div>

          <footer className="notification-bell__panel-footer">
            <Link
              to={`/users/${userKey}?tab=activity`}
              className="notification-bell__see-all"
              onClick={() => setOpen(false)}
            >
              See all activity →
            </Link>
          </footer>
        </div>
      )}
    </div>
  );
};

export default NotificationBell;
