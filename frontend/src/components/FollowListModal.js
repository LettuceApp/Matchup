import React, { useEffect } from 'react';
import { Link } from 'react-router-dom';
import Button from './Button';
import SkeletonRow from './SkeletonRow';
import '../styles/FollowListModal.css';

const FollowListModal = ({
  isOpen,
  onClose,
  activeTab,
  onTabChange,
  followers,
  following,
  followersLoading,
  followingLoading,
  followersError,
  followingError,
  followersCursor,
  followingCursor,
  onLoadMoreFollowers,
  onLoadMoreFollowing,
  onFollowToggle,
  listFollowBusyId,
  viewerId,
  resolveAvatarUrl,
  listFollowError,
}) => {
  useEffect(() => {
    if (!isOpen) return undefined;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = previousOverflow;
    };
  }, [isOpen]);

  if (!isOpen) return null;

  const activeList = activeTab === 'followers' ? followers : following;
  const activeLoading = activeTab === 'followers' ? followersLoading : followingLoading;
  const activeError = activeTab === 'followers' ? followersError : followingError;
  const activeCursor = activeTab === 'followers' ? followersCursor : followingCursor;
  const emptyHeading = activeError
    ? activeError
    : activeTab === 'followers'
      ? 'No followers yet'
      : 'Not following anyone yet';
  const emptyMessage = activeError
    ? 'Please try again later.'
    : activeTab === 'followers'
      ? 'No followers to show.'
      : 'No following to show.';

  return (
    <div className="follow-modal-overlay" onClick={onClose}>
      <div
        className="follow-modal"
        onClick={(event) => event.stopPropagation()}
      >
        <header className="follow-modal-header">
          <div>
            <h2>Connections</h2>
            <p>Manage followers and following.</p>
          </div>
          <button type="button" className="follow-modal-close" onClick={onClose}>
            Close
          </button>
        </header>

        <div className="follow-modal-tabs">
          <button
            type="button"
            className={`follow-modal-tab ${activeTab === 'followers' ? 'is-active' : ''}`}
            onClick={() => onTabChange('followers')}
          >
            Followers
          </button>
          <button
            type="button"
            className={`follow-modal-tab ${activeTab === 'following' ? 'is-active' : ''}`}
            onClick={() => onTabChange('following')}
          >
            Following
          </button>
        </div>

        {listFollowError && (
          <div className="follow-modal-error">{listFollowError}</div>
        )}

        <div className="follow-modal-list">
          {activeLoading ? (
            <div className="follow-modal-skeletons">
              {Array.from({ length: 4 }).map((_, index) => (
                <SkeletonRow key={`follow-skeleton-${index}`} />
              ))}
            </div>
          ) : activeList.length > 0 ? (
            activeList.map(person => {
              const avatarSrc = resolveAvatarUrl(person.avatar_path);
              const isSelf = viewerId && person?.id && viewerId === String(person.id);
              const profileSlug = person.username || person.id;
              return (
                <article key={person.id} className="follow-modal-card">
                  <div className="follow-modal-card-main">
                    <div className="follow-modal-avatar">
                      {avatarSrc ? (
                        <img src={avatarSrc} alt={`${person.username || 'User'} avatar`} />
                      ) : (
                        <span>MF</span>
                      )}
                    </div>
                    <div className="follow-modal-card-text">
                      <Link to={`/users/${profileSlug}`} className="follow-modal-name">
                        {person.username || 'Matchup Fan'}
                      </Link>
                      <p className="follow-modal-meta">
                        {(person.followers_count ?? 0).toLocaleString()} followers |{' '}
                        {(person.following_count ?? 0).toLocaleString()} following
                      </p>
                      <div className="follow-modal-badges">
                        {person.viewer_followed_by && (
                          <span className="follow-modal-badge">Follows you</span>
                        )}
                        {person.mutual && (
                          <span className="follow-modal-badge follow-modal-badge--mutual">
                            Mutual
                          </span>
                        )}
                      </div>
                    </div>
                  </div>
                  {!isSelf && (
                    <div className="follow-modal-card-actions">
                      <Button
                        onClick={() => onFollowToggle(person)}
                        className={person.viewer_following ? 'profile-secondary-button' : 'profile-primary-button'}
                        disabled={listFollowBusyId === person.id}
                      >
                        {person.viewer_following ? 'Following' : 'Follow'}
                      </Button>
                    </div>
                  )}
                </article>
              );
            })
          ) : (
            <div className="follow-modal-empty">
              <h3>{emptyHeading}</h3>
              <p>{emptyMessage}</p>
            </div>
          )}
        </div>

        {activeCursor && (
          <Button
            onClick={activeTab === 'followers' ? onLoadMoreFollowers : onLoadMoreFollowing}
            className="profile-secondary-button follow-modal-load"
            disabled={activeLoading}
          >
            {activeLoading ? 'Loading...' : 'Load more'}
          </Button>
        )}
      </div>
    </div>
  );
};

export default FollowListModal;
