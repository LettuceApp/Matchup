import React, { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import ProfilePic from '../components/ProfilePic';
import {
  listBlocks,
  listMutes,
  unblockUser,
  unmuteUser,
} from '../services/api';
import '../styles/BlocksAndMutes.css';

/*
 * BlocksAndMutes — `/settings/blocks`
 *
 * Lets the viewer see + revoke the block / mute edges they've set.
 * Two independent lists side by side (tabbed on mobile). Each entry
 * carries one button that inverts the current state; no edit mode or
 * bulk actions because the scale is bounded — users accumulate a
 * handful of blocks, not thousands.
 *
 * Server returns paginated UserProfile rows + next_cursor. We eagerly
 * concat on "Load more" rather than virtualising — the page is a
 * settings surface, not a feed, so the DOM cost stays trivial.
 */

const normalizeListResponse = (raw) => {
  if (!raw) return { users: [], nextCursor: null };
  const users = raw.users ?? raw.response?.users ?? [];
  const nextCursor = raw.next_cursor ?? raw.response?.next_cursor ?? null;
  return { users, nextCursor };
};

const UserRow = ({ user, actionLabel, onAction, busy }) => (
  <li className="blocks-list__row">
    <Link to={`/user/${user.username}`} className="blocks-list__link">
      <ProfilePic
        src={user.avatar_path}
        alt={user.username}
        size={44}
        className="blocks-list__avatar"
      />
      <div className="blocks-list__meta">
        <span className="blocks-list__name">{user.username}</span>
        {user.bio && <span className="blocks-list__bio">{user.bio}</span>}
      </div>
    </Link>
    <button
      type="button"
      className="blocks-list__action"
      onClick={() => onAction(user)}
      disabled={busy}
    >
      {actionLabel}
    </button>
  </li>
);

const BlocksAndMutes = () => {
  const [activeTab, setActiveTab] = useState('blocks');
  const [blocks, setBlocks] = useState([]);
  const [blocksCursor, setBlocksCursor] = useState(null);
  const [blocksLoading, setBlocksLoading] = useState(false);
  const [mutes, setMutes] = useState([]);
  const [mutesCursor, setMutesCursor] = useState(null);
  const [mutesLoading, setMutesLoading] = useState(false);
  const [error, setError] = useState(null);
  const [busyId, setBusyId] = useState(null);

  const loadBlocks = useCallback(async (cursor) => {
    setBlocksLoading(true);
    setError(null);
    try {
      const res = await listBlocks(cursor ? { cursor } : {});
      const { users, nextCursor } = normalizeListResponse(res.data);
      setBlocks((prev) => (cursor ? [...prev, ...users] : users));
      setBlocksCursor(nextCursor);
    } catch (err) {
      console.error('listBlocks failed', err);
      setError('Unable to load blocks.');
    } finally {
      setBlocksLoading(false);
    }
  }, []);

  const loadMutes = useCallback(async (cursor) => {
    setMutesLoading(true);
    setError(null);
    try {
      const res = await listMutes(cursor ? { cursor } : {});
      const { users, nextCursor } = normalizeListResponse(res.data);
      setMutes((prev) => (cursor ? [...prev, ...users] : users));
      setMutesCursor(nextCursor);
    } catch (err) {
      console.error('listMutes failed', err);
      setError('Unable to load mutes.');
    } finally {
      setMutesLoading(false);
    }
  }, []);

  useEffect(() => {
    // Lazy-load each tab on first view. Both are scoped to one viewer
    // so there's no cost to loading both up-front — the typical
    // settings visit eventually touches each.
    loadBlocks();
    loadMutes();
  }, [loadBlocks, loadMutes]);

  const handleUnblock = async (user) => {
    setBusyId(user.username);
    try {
      await unblockUser(user.username);
      setBlocks((prev) => prev.filter((u) => u.username !== user.username));
    } catch (err) {
      console.error('unblock failed', err);
      setError(`Couldn't unblock ${user.username}.`);
    } finally {
      setBusyId(null);
    }
  };

  const handleUnmute = async (user) => {
    setBusyId(user.username);
    try {
      await unmuteUser(user.username);
      setMutes((prev) => prev.filter((u) => u.username !== user.username));
    } catch (err) {
      console.error('unmute failed', err);
      setError(`Couldn't unmute ${user.username}.`);
    } finally {
      setBusyId(null);
    }
  };

  return (
    <div className="blocks-page">
      <main className="blocks-page__main">
        <header className="blocks-page__header">
          <h1>Blocks &amp; mutes</h1>
          <p>
            Blocks are bidirectional — the user can't see your content and
            you can't see theirs. Mutes are one-way, hiding their posts
            from your activity feed only.
          </p>
        </header>

        <div className="blocks-page__tabs" role="tablist">
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === 'blocks'}
            className={`blocks-page__tab ${activeTab === 'blocks' ? 'blocks-page__tab--active' : ''}`}
            onClick={() => setActiveTab('blocks')}
          >
            Blocked ({blocks.length})
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === 'mutes'}
            className={`blocks-page__tab ${activeTab === 'mutes' ? 'blocks-page__tab--active' : ''}`}
            onClick={() => setActiveTab('mutes')}
          >
            Muted ({mutes.length})
          </button>
        </div>

        {error && <div className="blocks-page__error">{error}</div>}

        {activeTab === 'blocks' ? (
          <section>
            {blocks.length === 0 && !blocksLoading ? (
              <div className="blocks-page__empty">
                You haven't blocked anyone.
              </div>
            ) : (
              <ul className="blocks-list">
                {blocks.map((u) => (
                  <UserRow
                    key={u.username}
                    user={u}
                    actionLabel="Unblock"
                    onAction={handleUnblock}
                    busy={busyId === u.username}
                  />
                ))}
              </ul>
            )}
            {blocksCursor && (
              <button
                type="button"
                className="blocks-page__more"
                onClick={() => loadBlocks(blocksCursor)}
                disabled={blocksLoading}
              >
                Load more
              </button>
            )}
          </section>
        ) : (
          <section>
            {mutes.length === 0 && !mutesLoading ? (
              <div className="blocks-page__empty">
                You haven't muted anyone.
              </div>
            ) : (
              <ul className="blocks-list">
                {mutes.map((u) => (
                  <UserRow
                    key={u.username}
                    user={u}
                    actionLabel="Unmute"
                    onAction={handleUnmute}
                    busy={busyId === u.username}
                  />
                ))}
              </ul>
            )}
            {mutesCursor && (
              <button
                type="button"
                className="blocks-page__more"
                onClick={() => loadMutes(mutesCursor)}
                disabled={mutesLoading}
              >
                Load more
              </button>
            )}
          </section>
        )}
      </main>
    </div>
  );
};

export default BlocksAndMutes;
