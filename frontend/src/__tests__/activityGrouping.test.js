import { groupActivityItems, unreadCountFromGrouped } from '../utils/activityGrouping';

// Anchor every test against a fixed "now" so the 24h-window math is
// deterministic and isn't a function of when the test runs.
const NOW = Date.UTC(2026, 3, 21, 12, 0, 0);
const isoMinutesAgo = (n) => new Date(NOW - n * 60_000).toISOString();
const isoHoursAgo = (n) => new Date(NOW - n * 3600_000).toISOString();

// Tiny builder — defaults that satisfy groupActivityItems' preconditions.
const item = (overrides) => ({
  id: 'x',
  kind: 'like_received',
  occurred_at: isoMinutesAgo(1),
  subject_id: 'subject-1',
  subject_type: 'matchup',
  subject_title: 'Thing',
  actor_username: 'alice',
  ...overrides,
});

describe('groupActivityItems', () => {
  it('returns an empty array for empty input', () => {
    expect(groupActivityItems([])).toEqual([]);
  });

  it('passes solo items through unchanged', () => {
    const input = [item({ id: 'a' }), item({ id: 'b', subject_id: 'subject-2' })];
    const out = groupActivityItems(input);
    expect(out).toHaveLength(2);
    expect(out[0].group_count).toBeUndefined();
    expect(out[1].group_count).toBeUndefined();
  });

  it('folds two same-subject same-kind items within window into one group', () => {
    const input = [
      item({ id: '1', actor_username: 'alice', occurred_at: isoMinutesAgo(5) }),
      item({ id: '2', actor_username: 'bob', occurred_at: isoMinutesAgo(30) }),
    ];
    const out = groupActivityItems(input);
    expect(out).toHaveLength(1);
    expect(out[0].group_count).toBe(2);
    expect(out[0].group_actor_usernames).toEqual(['alice', 'bob']);
    // Head keeps the newest occurred_at so relativeTime reads correctly.
    expect(out[0].occurred_at).toBe(input[0].occurred_at);
  });

  it('keeps two same-key items OUTSIDE the window as separate rows', () => {
    const input = [
      item({ id: '1', occurred_at: isoMinutesAgo(1) }),
      item({ id: '2', occurred_at: isoHoursAgo(30) }), // > 24h older
    ];
    const out = groupActivityItems(input);
    expect(out).toHaveLength(2);
    expect(out[0].group_count).toBeUndefined();
    expect(out[1].group_count).toBeUndefined();
  });

  it('treats different kinds on the same subject as separate groups', () => {
    const input = [
      item({ id: '1', kind: 'like_received' }),
      item({ id: '2', kind: 'comment_received' }),
    ];
    const out = groupActivityItems(input);
    expect(out).toHaveLength(2);
  });

  it('treats different subjects with same kind as separate groups', () => {
    const input = [
      item({ id: '1', subject_id: 'a' }),
      item({ id: '2', subject_id: 'b' }),
    ];
    const out = groupActivityItems(input);
    expect(out).toHaveLength(2);
  });

  it('passes through items lacking subject_id without grouping', () => {
    const input = [
      item({ id: '1', subject_id: null }),
      item({ id: '2', subject_id: null }),
    ];
    const out = groupActivityItems(input);
    expect(out).toHaveLength(2);
  });

  it('deduplicates repeat actors inside a group', () => {
    const input = [
      item({ id: '1', actor_username: 'alice' }),
      item({ id: '2', actor_username: 'alice' }),
      item({ id: '3', actor_username: 'alice' }),
    ];
    const out = groupActivityItems(input);
    expect(out).toHaveLength(1);
    expect(out[0].group_count).toBe(3);
    expect(out[0].group_actor_usernames).toEqual(['alice']);
  });

  it('does not mutate the input array or its members', () => {
    const input = [item({ id: '1' }), item({ id: '2' })];
    const snapshot = JSON.parse(JSON.stringify(input));
    groupActivityItems(input);
    expect(input).toEqual(snapshot);
  });

  it('respects a custom windowHours option', () => {
    const input = [
      item({ id: '1', occurred_at: isoMinutesAgo(1) }),
      item({ id: '2', occurred_at: isoHoursAgo(3) }),
    ];
    // Default 24h window: groups together.
    expect(groupActivityItems(input)).toHaveLength(1);
    // Narrow 1h window: keeps them separate.
    expect(groupActivityItems(input, { windowHours: 1 })).toHaveLength(2);
  });
});

describe('unreadCountFromGrouped', () => {
  it('returns 0 for an empty list', () => {
    expect(unreadCountFromGrouped([], null)).toBe(0);
  });

  it('counts all items as unread when lastSeenAt is null', () => {
    const input = [
      item({ id: '1' }),
      item({ id: '2', subject_id: 'subject-2' }),
    ];
    expect(unreadCountFromGrouped(input, null)).toBe(2);
  });

  it('excludes items older than lastSeenAt', () => {
    const input = [
      item({ id: '1', occurred_at: isoMinutesAgo(1) }),
      item({ id: '2', occurred_at: isoHoursAgo(48) }),
    ];
    const lastSeen = new Date(NOW - 2 * 3600_000).toISOString();
    expect(unreadCountFromGrouped(input, lastSeen)).toBe(1);
  });

  it('counts a grouped item as 1 regardless of group_count', () => {
    const input = [
      {
        ...item({ id: '1', occurred_at: isoMinutesAgo(1) }),
        group_count: 50,
        group_actor_usernames: ['a', 'b', 'c'],
      },
    ];
    expect(unreadCountFromGrouped(input, null)).toBe(1);
  });

  it('skips items with read_at set (persisted notification stamped)', () => {
    const input = [
      // Stamped persisted row → read, must not count.
      item({
        id: '1',
        kind: 'milestone_reached',
        occurred_at: isoMinutesAgo(1),
        read_at: isoMinutesAgo(0.5),
      }),
      // Unstamped persisted row → unread.
      item({
        id: '2',
        kind: 'milestone_reached',
        occurred_at: isoMinutesAgo(1),
        subject_id: 'subject-2',
      }),
      // Derived kind (no read_at) → unread via lastSeenAt fallback.
      item({
        id: '3',
        kind: 'vote_cast',
        occurred_at: isoMinutesAgo(1),
        subject_id: 'subject-3',
      }),
    ];
    // null lastSeenAt → fallback counts everything without read_at.
    expect(unreadCountFromGrouped(input, null)).toBe(2);
  });
});
