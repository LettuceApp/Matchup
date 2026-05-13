import React, { useCallback, useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import {
  banCommunityMember,
  getCommunityBySlug,
  listCommunityMembers,
  removeCommunityMember,
  unbanCommunityMember,
  updateCommunityMemberRole,
} from '../services/api';
import '../styles/CommunityMembers.css';

/*
 * CommunityMembers — paginated member list with role-based actions.
 *
 * Visibility:
 *   - Public read: anyone can see the member list (matches Reddit /
 *     Discord defaults). Bans are filtered server-side unless the
 *     viewer is a mod+; mods see banned members so they can unban.
 *
 * Actions (gated by viewer_role — backend re-checks):
 *   - Owner: promote/demote member ↔ mod, remove, ban / unban
 *   - Mod: remove, ban / unban (cannot touch the owner or other mods)
 *   - Member / anon: read-only
 */
const ROLE_LABELS = {
  owner: 'Owner',
  mod: 'Mod',
  member: 'Member',
  banned: 'Banned',
};

const CommunityMembers = () => {
  const { slug } = useParams();
  const [community, setCommunity] = useState(null);
  const [communityLoading, setCommunityLoading] = useState(true);
  const [error, setError] = useState(null);

  const [members, setMembers] = useState([]);
  const [membersLoading, setMembersLoading] = useState(false);
  const [showBanned, setShowBanned] = useState(false);
  const [pendingActionUserId, setPendingActionUserId] = useState(null);

  const reloadMembers = useCallback(
    async (community) => {
      if (!community?.id) return;
      setMembersLoading(true);
      try {
        const res = await listCommunityMembers({
          communityId: community.id,
          limit: 100,
          role: showBanned ? 'banned' : '',
        });
        setMembers(res?.data?.members || []);
      } catch (err) {
        console.warn('Member load failed', err);
      } finally {
        setMembersLoading(false);
      }
    },
    [showBanned],
  );

  useEffect(() => {
    let cancelled = false;
    setCommunityLoading(true);
    (async () => {
      try {
        const res = await getCommunityBySlug(slug);
        if (cancelled) return;
        const c = res?.data?.community ?? null;
        setCommunity(c);
        await reloadMembers(c);
      } catch (err) {
        if (cancelled) return;
        setError(err?.response?.status === 404 ? 'Community not found.' : 'Could not load this community.');
      } finally {
        if (!cancelled) setCommunityLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [slug, reloadMembers]);

  const viewerRole = community?.viewer_role || '';
  const isOwner = viewerRole === 'owner';
  const isMod = viewerRole === 'mod' || isOwner;

  const handleRoleChange = async (userId, nextRole) => {
    if (!community) return;
    setPendingActionUserId(userId);
    try {
      await updateCommunityMemberRole({ communityId: community.id, userId, role: nextRole });
      await reloadMembers(community);
    } catch (err) {
      console.warn('Role change failed', err);
      alert('Could not change role.');
    } finally {
      setPendingActionUserId(null);
    }
  };

  const handleRemove = async (userId) => {
    if (!community) return;
    if (!window.confirm('Remove this member from the community? They can re-join.')) return;
    setPendingActionUserId(userId);
    try {
      await removeCommunityMember({ communityId: community.id, userId });
      await reloadMembers(community);
    } catch (err) {
      console.warn('Remove failed', err);
      alert('Could not remove member.');
    } finally {
      setPendingActionUserId(null);
    }
  };

  const handleBan = async (userId) => {
    if (!community) return;
    const reason = window.prompt('Optional reason for the ban (visible to mods only):', '');
    if (reason === null) return; // user cancelled
    setPendingActionUserId(userId);
    try {
      await banCommunityMember({ communityId: community.id, userId, reason: reason.trim() });
      await reloadMembers(community);
    } catch (err) {
      console.warn('Ban failed', err);
      alert('Could not ban member.');
    } finally {
      setPendingActionUserId(null);
    }
  };

  const handleUnban = async (userId) => {
    if (!community) return;
    setPendingActionUserId(userId);
    try {
      await unbanCommunityMember({ communityId: community.id, userId });
      await reloadMembers(community);
    } catch (err) {
      console.warn('Unban failed', err);
      alert('Could not unban.');
    } finally {
      setPendingActionUserId(null);
    }
  };

  if (communityLoading) {
    return (
      <div className="community-members-page community-members-page--loading">
        <div className="community-members__spinner" aria-label="Loading members" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="community-members-page community-members-page--error">
        <h1>{error}</h1>
        <Link to="/home">← Back to home</Link>
      </div>
    );
  }

  if (!community) return null;

  return (
    <div className="community-members-page">
      <header className="community-members__header">
        <div>
          <p className="community-members__overline">
            <Link to={`/c/${community.slug}`}>/c/{community.slug}</Link>
          </p>
          <h1 className="community-members__title">Members</h1>
          <p className="community-members__subtitle">
            <strong>{community.member_count}</strong>{' '}
            {community.member_count === 1 ? 'member' : 'members'}
          </p>
        </div>
        {isMod && (
          <div className="community-members__tabs">
            <button
              type="button"
              className={`community-members__tab ${!showBanned ? 'is-active' : ''}`}
              onClick={() => setShowBanned(false)}
            >
              Members
            </button>
            <button
              type="button"
              className={`community-members__tab ${showBanned ? 'is-active' : ''}`}
              onClick={() => setShowBanned(true)}
            >
              Banned
            </button>
          </div>
        )}
      </header>

      {membersLoading && members.length === 0 ? (
        <div className="community-members__empty">Loading…</div>
      ) : members.length === 0 ? (
        <div className="community-members__empty">
          {showBanned ? 'No banned members.' : 'No members yet.'}
        </div>
      ) : (
        <ul className="community-members__list">
          {members.map((m) => {
            const pending = pendingActionUserId === m.id;
            const canTouch = isMod && m.role !== 'owner';
            const canPromote = isOwner && m.role === 'member';
            const canDemote = isOwner && m.role === 'mod';
            return (
              <li key={m.id} className="community-members__item">
                <Link to={`/users/${m.username}`} className="community-members__user">
                  <div className="community-members__avatar">
                    {m.avatar_path ? (
                      <img src={m.avatar_path} alt="" />
                    ) : (
                      <span>{(m.username || '?').charAt(0).toUpperCase()}</span>
                    )}
                  </div>
                  <div className="community-members__user-text">
                    <span className="community-members__username">@{m.username}</span>
                    <span className={`community-members__role-pill is-${m.role}`}>
                      {ROLE_LABELS[m.role] || m.role}
                    </span>
                  </div>
                </Link>

                {canTouch && (
                  <div className="community-members__actions">
                    {canPromote && (
                      <button
                        type="button"
                        className="community-members__btn"
                        onClick={() => handleRoleChange(m.id, 'mod')}
                        disabled={pending}
                      >
                        Promote to mod
                      </button>
                    )}
                    {canDemote && (
                      <button
                        type="button"
                        className="community-members__btn"
                        onClick={() => handleRoleChange(m.id, 'member')}
                        disabled={pending}
                      >
                        Demote to member
                      </button>
                    )}
                    {m.role !== 'banned' && (
                      <button
                        type="button"
                        className="community-members__btn"
                        onClick={() => handleRemove(m.id)}
                        disabled={pending}
                      >
                        Remove
                      </button>
                    )}
                    {m.role !== 'banned' ? (
                      <button
                        type="button"
                        className="community-members__btn community-members__btn--danger"
                        onClick={() => handleBan(m.id)}
                        disabled={pending}
                      >
                        Ban
                      </button>
                    ) : (
                      <button
                        type="button"
                        className="community-members__btn"
                        onClick={() => handleUnban(m.id)}
                        disabled={pending}
                      >
                        Unban
                      </button>
                    )}
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
};

export default CommunityMembers;
