/*
 * groupActivityItems — collapses repeat (subject_id, kind) events from
 * the activity feed so the NotificationBell doesn't drown in "9+" on
 * a viral matchup. Granular events are preserved for the Activity tab
 * (this helper is opt-in per callsite).
 *
 * Grouping rule: two items fold together when they share BOTH
 * `subject_id` AND `kind`, AND the newer item's `occurred_at` is within
 * `windowHours` of the OLDEST item already in that group's window.
 * Items arrive sorted DESC by occurred_at; we walk once, keeping the
 * first occurrence of each key as the group head and merging
 * newer-or-same-age siblings into it.
 *
 * The grouped item attaches:
 *   group_count            total events folded in (>= 2 means "render as group")
 *   group_actor_usernames  unique actors across the group, newest first
 *
 * We deliberately do NOT mutate `kind` — the renderer branches on
 * `group_count > 1` to pick a grouped copy template. That keeps the
 * icon + accent mapping in one place (no parallel `*_grouped` keys to
 * maintain in the CSS).
 *
 * Input is never mutated — every grouped output is a shallow clone.
 */

const DEFAULT_WINDOW_HOURS = 24;

export function groupActivityItems(items, { windowHours = DEFAULT_WINDOW_HOURS } = {}) {
  if (!Array.isArray(items)) return [];
  const windowMs = windowHours * 3600 * 1000;

  const out = [];
  // key: `${kind}:${subject_id}` -> index into `out` of the group head
  const groupIdx = new Map();

  for (const raw of items) {
    // Items missing a subject_id can't be deduped meaningfully — pass
    // them through. Same for items missing a kind.
    if (!raw || !raw.subject_id || !raw.kind) {
      out.push(raw);
      continue;
    }

    const key = `${raw.kind}:${raw.subject_id}`;
    const idx = groupIdx.get(key);

    if (idx === undefined) {
      // First time seeing this key — push a shallow clone so we can
      // attach group_* fields later without mutating the caller's
      // array element.
      groupIdx.set(key, out.length);
      out.push({ ...raw });
      continue;
    }

    const head = out[idx];
    const headTs = Date.parse(head.occurred_at);
    const nowTs = Date.parse(raw.occurred_at);

    // Since input is sorted DESC, head is newer-or-equal vs. the
    // current item. If the current item is outside the window from
    // the head, it belongs to a separate, older group — push as a
    // distinct row and shift the group pointer to this new head.
    if (!Number.isFinite(headTs) || !Number.isFinite(nowTs) || headTs - nowTs > windowMs) {
      groupIdx.set(key, out.length);
      out.push({ ...raw });
      continue;
    }

    // Fold into the existing group head.
    if (!head.group_count) {
      head.group_count = 1; // the head itself counts
      head.group_actor_usernames = head.actor_username ? [head.actor_username] : [];
    }
    head.group_count += 1;
    if (
      raw.actor_username &&
      !head.group_actor_usernames.includes(raw.actor_username)
    ) {
      head.group_actor_usernames.push(raw.actor_username);
    }
  }

  return out;
}

/*
 * unreadCountFromGrouped — the badge count we show on the bell.
 * Grouped items count as 1 regardless of how many raw events they
 * represent, so a viral matchup doesn't peg the badge at "9+".
 *
 * Hybrid read-state: persisted notifications (milestone_reached,
 * matchup_closing_soon, tie_needs_resolution) carry a server-side
 * `read_at` — if set, the item is read regardless of lastSeenAt.
 * Derived kinds (vote_cast, like_received, etc.) don't have a row
 * to stamp, so they fall back to the lastSeenAt timestamp.
 */
export function unreadCountFromGrouped(items, lastSeenAt) {
  if (!Array.isArray(items) || items.length === 0) return 0;
  const seen = lastSeenAt ? Date.parse(lastSeenAt) : 0;
  let n = 0;
  for (const item of items) {
    if (item.read_at) continue; // persisted row already stamped
    const t = Date.parse(item.occurred_at);
    if (Number.isFinite(t) && t > seen) n += 1;
  }
  return n;
}
