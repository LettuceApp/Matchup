import React from 'react';
import { Link } from 'react-router-dom';
import {
  FiZap,
  FiAward,
  FiArrowDown,
  FiTrendingUp,
  FiHeart,
  FiMessageCircle,
  FiUserPlus,
  FiCheckCircle,
  FiAtSign,
  FiFlag,
  FiStar,
  FiClock,
  FiAlertTriangle,
  FiEdit3,
} from 'react-icons/fi';
import { relativeTime } from '../utils/time';
import '../styles/ActivityFeed.css';

/*
 * ActivityFeed — renders the 8 notification kinds returned by the
 * ActivityService RPC. The backend is deliberately dumb about copy
 * (kind + payload only); this component owns all user-facing strings
 * so we can iterate on tone without a proto change.
 *
 * Visual grammar:
 *   [icon]  [verb phrase] · [2h ago]
 *           [subject title, linkified]
 *           (optional) [body snippet / payload detail]
 *
 * `lastSeenAt` is an ISO timestamp string from localStorage; items
 * newer than that render a small unread dot. The parent (UserProfile)
 * advances `lastSeenAt` to now when the tab opens.
 */

// Map kind → icon + accent class. Centralised so the render branch
// stays a switch on text rather than a prop-drilled JSX tree.
const KIND_META = {
  vote_cast: { Icon: FiZap, className: 'activity-item--cast' },
  vote_win: { Icon: FiAward, className: 'activity-item--win' },
  vote_loss: { Icon: FiArrowDown, className: 'activity-item--loss' },
  bracket_progress: { Icon: FiTrendingUp, className: 'activity-item--progress' },
  matchup_vote_received: { Icon: FiCheckCircle, className: 'activity-item--received' },
  like_received: { Icon: FiHeart, className: 'activity-item--like' },
  comment_received: { Icon: FiMessageCircle, className: 'activity-item--comment' },
  new_follower: { Icon: FiUserPlus, className: 'activity-item--follow' },
  mention_received: { Icon: FiAtSign, className: 'activity-item--mention' },
  matchup_completed: { Icon: FiFlag, className: 'activity-item--completed-matchup' },
  bracket_completed: { Icon: FiAward, className: 'activity-item--completed-bracket' },
  milestone_reached: { Icon: FiStar, className: 'activity-item--milestone' },
  matchup_closing_soon: { Icon: FiClock, className: 'activity-item--closing' },
  tie_needs_resolution: { Icon: FiAlertTriangle, className: 'activity-item--tie' },
  followed_user_posted: { Icon: FiEdit3, className: 'activity-item--followed-posted' },
};

// Grouped renderers — used when NotificationBell collapses repeat
// (subject_id, kind) events into one row via groupActivityItems. The
// `item.group_count` and `item.group_actor_usernames` fields are
// attached by the grouping helper; render switches based on count
// >= 2. Only kinds that make sense to stack have an entry here —
// everything else falls through to RENDERERS below.
const GROUP_RENDERERS = {
  matchup_vote_received: ({ item }) => (
    <>
      <strong>{item.group_count} votes</strong> on <SubjectLink item={item} />.
    </>
  ),
  like_received: ({ item }) => (
    <>
      <strong>{item.group_count} people</strong> liked <SubjectLink item={item} />
      {item.group_actor_usernames?.length > 0 && (
        <span className="activity-item__muted">
          {' — including '}
          <ActorLink item={{ actor_username: item.group_actor_usernames[0] }} />
        </span>
      )}
      .
    </>
  ),
  comment_received: ({ item }) => (
    <>
      <strong>{item.group_count} comments</strong> on <SubjectLink item={item} />.
    </>
  ),
};

// Each kind owns a render function for its verb phrase. Keeps the
// switch statement readable and makes it trivial to add new kinds.
const RENDERERS = {
  vote_cast: ({ item }) => (
    <>
      You voted{' '}
      {item.payload?.voted_item && (
        <>
          for <strong>{item.payload.voted_item}</strong>{' '}
        </>
      )}
      in <SubjectLink item={item} />.
    </>
  ),
  vote_win: ({ item }) => (
    <>
      🏆 You backed <strong>{item.payload?.voted_item}</strong> — won{' '}
      <SubjectLink item={item} />.
    </>
  ),
  vote_loss: ({ item }) => (
    <>
      Your pick <strong>{item.payload?.voted_item}</strong> didn’t take{' '}
      <SubjectLink item={item} />.{' '}
      {item.payload?.winner_item && (
        <span className="activity-item__muted">
          {item.payload.winner_item} won.
        </span>
      )}
    </>
  ),
  bracket_progress: ({ item }) => {
    const status = item.payload?.status;
    const round = item.payload?.round;
    if (status === 'completed') {
      return (
        <>
          <SubjectLink item={item} /> crowned a champion.
        </>
      );
    }
    return (
      <>
        <SubjectLink item={item} /> advanced
        {round ? (
          <>
            {' '}to <strong>Round {round}</strong>
          </>
        ) : null}
        .
      </>
    );
  },
  matchup_vote_received: ({ item }) => (
    <>
      Someone voted on <SubjectLink item={item} />
      {item.payload?.item && (
        <>
          {' '}(<strong>{item.payload.item}</strong>)
        </>
      )}
      .
    </>
  ),
  like_received: ({ item }) => (
    <>
      <ActorLink item={item} /> liked <SubjectLink item={item} />.
    </>
  ),
  comment_received: ({ item }) => (
    <>
      <ActorLink item={item} /> commented on <SubjectLink item={item} />.
      {item.payload?.body && (
        <div className="activity-item__snippet">“{item.payload.body}”</div>
      )}
    </>
  ),
  new_follower: ({ item }) => (
    <>
      <ActorLink item={item} /> followed you.
    </>
  ),
  mention_received: ({ item }) => (
    <>
      <ActorLink item={item} /> mentioned you in <SubjectLink item={item} />.
      {item.payload?.body && (
        <div className="activity-item__snippet">“{item.payload.body}”</div>
      )}
    </>
  ),
  matchup_completed: ({ item }) => (
    <>
      🏁 Your matchup <SubjectLink item={item} /> closed
      {item.payload?.winner_item ? (
        <>
          {' — '}<strong>{item.payload.winner_item}</strong> won.
        </>
      ) : (
        '.'
      )}
    </>
  ),
  bracket_completed: ({ item }) => (
    <>
      🏁 Your tournament <SubjectLink item={item} /> crowned a champion.
    </>
  ),
  milestone_reached: ({ item }) => {
    const metric = item.payload?.metric || 'engagement';
    const threshold = item.payload?.threshold;
    const metricLabel = {
      votes: 'votes',
      likes: 'likes',
      followers: 'followers',
    }[metric] || metric;
    // "votes" / "likes" attach to a matchup or bracket subject;
    // "followers" attaches to the user themselves (copy reads
    // "You hit 100 followers").
    if (item.subject_type === 'user') {
      return (
        <>
          🎯 You hit <strong>{threshold}</strong> {metricLabel}!
        </>
      );
    }
    return (
      <>
        🎯 <SubjectLink item={item} /> hit <strong>{threshold}</strong> {metricLabel}!
      </>
    );
  },
  matchup_closing_soon: ({ item }) => {
    // role flag lets us pick the right CTA — authors finalize, voters
    // lock in their pick. Legacy rows without a role default to 'author'
    // because that's the only shape the scanner emitted before #8 added
    // the voter pass.
    const isVoter = item.payload?.role === 'voter';
    return (
      <>
        ⏰ <SubjectLink item={item} /> closes in ~1 hour —{' '}
        {isVoter ? 'lock in your pick before it ends.' : 'ready to finalize?'}
      </>
    );
  },
  tie_needs_resolution: ({ item }) => (
    <>
      ⚠ <SubjectLink item={item} /> ended in a tie — pick a winner.
    </>
  ),
  followed_user_posted: ({ item }) => (
    <>
      <ActorLink item={item} /> posted a new{' '}
      {item.subject_type === 'bracket' ? 'tournament' : 'matchup'}:{' '}
      <SubjectLink item={item} />.
    </>
  ),
};

function SubjectLink({ item }) {
  const path = subjectPath(item);
  const label = item.subject_title || 'this';
  if (!path) return <span>{label}</span>;
  return (
    <Link to={path} className="activity-item__subject-link">
      {label}
    </Link>
  );
}

function ActorLink({ item }) {
  const name = item.actor_username;
  if (!name) return <span>Someone</span>;
  return (
    <Link to={`/users/${name}`} className="activity-item__actor-link">
      @{name}
    </Link>
  );
}

// Maps an activity item to its canonical SPA route. The subject_id is
// the matchup/bracket public UUID — NOT the short_id, and NOT a route
// the React app has any handler for — so linking to `/m/{uuid}` used to
// fall through to the SPA catch-all and bounce the user home. The
// backend now emits `author_username` on every matchup-typed payload,
// so we can build the real route (`/users/{author}/matchup/{id}`).
// Brackets already work because `/brackets/{id}` is a live SPA route.
function subjectPath(item) {
  if (!item?.subject_id) return null;
  switch (item.subject_type) {
    case 'matchup': {
      const author = item.payload?.author_username;
      if (!author) return null;
      return `/users/${author}/matchup/${item.subject_id}`;
    }
    case 'bracket':
      return `/brackets/${item.subject_id}`;
    case 'user': {
      // Prefer actor_username (follow events etc.); fall back to
      // payload.title (milestone_reached stores username there) and
      // payload.author_username (self-subject events) so user-subject
      // links always resolve even when the event has no actor.
      const name =
        item.actor_username ||
        item.payload?.title ||
        item.payload?.author_username;
      return name ? `/users/${name}` : null;
    }
    default:
      return null;
  }
}

// Hybrid unread check: persisted notifications use server-side
// `read_at` as the source of truth; derived kinds fall back to
// the client's activity_last_seen_at timestamp. Null lastSeenAt
// keeps the original "no dots until we've stamped once" behavior
// so we don't flash every row as unread on first panel open.
function isNewer(item, lastSeenAt) {
  if (item.read_at) return false;
  if (!lastSeenAt) return false;
  const seen = new Date(lastSeenAt).getTime();
  const occurred = new Date(item.occurred_at).getTime();
  return Number.isFinite(seen) && Number.isFinite(occurred) && occurred > seen;
}

const ActivityFeed = ({
  items = [],
  lastSeenAt = null,
  loading = false,
  error = null,
  onRetry = null,
}) => {
  if (loading) {
    return <div className="activity-feed__state">Loading activity…</div>;
  }
  if (error) {
    // Keep the error copy quiet, but offer a retry affordance when the
    // parent supplies one — otherwise the 60s poll loop silently
    // recovers but the user has no way to know that or hasten it.
    return (
      <div className="activity-feed__state activity-feed__state--error">
        <span>{error}</span>
        {typeof onRetry === 'function' && (
          <button
            type="button"
            className="activity-feed__retry"
            onClick={onRetry}
          >
            Retry
          </button>
        )}
      </div>
    );
  }
  if (!items.length) {
    return (
      <div className="activity-feed__state">
        <p className="activity-feed__empty-title">Nothing yet</p>
        <p className="activity-feed__empty-body">
          Vote on a matchup, follow a creator, or share your own — your activity will show up here.
        </p>
      </div>
    );
  }

  return (
    <ul className="activity-feed">
      {items.map((item) => {
        const meta = KIND_META[item.kind] || KIND_META.vote_cast;
        // Grouped items use GROUP_RENDERERS when available; single
        // items (or grouped kinds without a group renderer) fall back
        // to the standard RENDERERS map.
        const grouped = item.group_count && item.group_count > 1;
        const Renderer =
          (grouped && GROUP_RENDERERS[item.kind]) || RENDERERS[item.kind];
        const unread = isNewer(item, lastSeenAt);
        return (
          <li
            key={item.id}
            className={`activity-item ${meta.className} ${unread ? 'is-unread' : ''}`}
          >
            <span className="activity-item__icon" aria-hidden="true">
              <meta.Icon />
            </span>
            <div className="activity-item__body">
              <p className="activity-item__message">
                {Renderer ? <Renderer item={item} /> : <em>{item.kind}</em>}
              </p>
              <p className="activity-item__time">{relativeTime(item.occurred_at)}</p>
            </div>
            {unread && <span className="activity-item__dot" aria-label="New" />}
          </li>
        );
      })}
    </ul>
  );
};

export default ActivityFeed;
