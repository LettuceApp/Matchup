import React, { useState, useEffect, useMemo } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import { AnimatePresence, motion } from 'framer-motion';
import * as Tabs from '@radix-ui/react-tabs';
import {
  FiZap,
  FiStar,
  FiTrendingUp,
  FiMessageCircle,
  FiActivity,
} from 'react-icons/fi';
import {
  getUserMatchups,
  getUserBrackets,
  getUser,
  archiveBracket,
  followUser,
  unfollowUser,
  getUserRelationship,
  getUserFollowers,
  getUserFollowing,
  updateUserPrivacy,
  getUserLikes,
  getUserBracketLikes,
  getMatchup,
  getBracket,
} from '../services/api';

import NavigationBar from '../components/NavigationBar';
import ProfilePic from '../components/ProfilePic';
import Button from '../components/Button';
import FollowListModal from '../components/FollowListModal';
import EmptyStateCard from '../components/EmptyStateCard';
import SkeletonCard from '../components/SkeletonCard';
import '../styles/UserProfile.css';

const normalizeToArray = (raw) => {
  if (Array.isArray(raw)) return raw;

  if (raw && typeof raw === 'object') {
    if (Array.isArray(raw.response)) return raw.response;
    if (Array.isArray(raw.data)) return raw.data;
    if (Array.isArray(raw.matchups)) return raw.matchups;
    if (Array.isArray(raw.brackets)) return raw.brackets;

    if (raw.response && typeof raw.response === 'object') {
      if (Array.isArray(raw.response.matchups)) return raw.response.matchups;
      if (Array.isArray(raw.response.brackets)) return raw.response.brackets;
      if (Array.isArray(raw.response.data)) return raw.response.data;
    }
  }

  return [];
};

const extractLikeMatchupId = (like = {}) =>
  like.matchup_id ??
  like.matchupId ??
  like.matchup_public_id ??
  like.matchupPublicId ??
  null;

const extractLikeBracketId = (like = {}) =>
  like.bracket_id ??
  like.bracketId ??
  like.bracket_public_id ??
  like.bracketPublicId ??
  null;

const extractBracketId = (matchup = {}) =>
  matchup.bracket_id ??
  matchup.bracketId ??
  matchup.BracketID ??
  matchup.bracketID ??
  null;

const isStandaloneMatchup = (matchup = {}) => extractBracketId(matchup) === null;

const S3_BUCKET = process.env.REACT_APP_S3_BUCKET;
const AWS_REGION = process.env.REACT_APP_AWS_REGION;
const S3_BASE_URL = process.env.REACT_APP_S3_BASE;

const resolveAvatarUrl = (value) => {
  if (!value) return null;
  if (value.startsWith('http')) return value;
  if (S3_BASE_URL) return `${S3_BASE_URL}/${value}`;
  if (S3_BUCKET && AWS_REGION) {
    return `https://${S3_BUCKET}.s3.${AWS_REGION}.amazonaws.com/${value}`;
  }
  return value;
};

const formatStat = (value) => {
  if (value === null || value === undefined) return '--';
  if (Number(value) === 0) return '--';
  return value;
};

const UserProfile = () => {
  const { username, userId } = useParams();
  const identifier = username || userId;
  const navigate = useNavigate();

  const [user, setUser] = useState(null);
  const [matchups, setMatchups] = useState([]);
  const [brackets, setBrackets] = useState([]);
  const [followers, setFollowers] = useState([]);
  const [followersCursor, setFollowersCursor] = useState(null);
  const [followersLoaded, setFollowersLoaded] = useState(false);
  const [followersLoading, setFollowersLoading] = useState(false);
  const [followersError, setFollowersError] = useState(null);
  const [following, setFollowing] = useState([]);
  const [followingCursor, setFollowingCursor] = useState(null);
  const [followingLoaded, setFollowingLoaded] = useState(false);
  const [followingLoading, setFollowingLoading] = useState(false);
  const [followingError, setFollowingError] = useState(null);
  const [activeFollowTab, setActiveFollowTab] = useState('followers');
  const [relationship, setRelationship] = useState(null);
  const [followBusy, setFollowBusy] = useState(false);
  const [followError, setFollowError] = useState(null);
  const [privacyUpdating, setPrivacyUpdating] = useState(false);
  const [privacyError, setPrivacyError] = useState(null);
  const [listFollowBusyId, setListFollowBusyId] = useState(null);
  const [listFollowError, setListFollowError] = useState(null);
  const [followModalOpen, setFollowModalOpen] = useState(false);
  const [activeTab, setActiveTab] = useState('matchups');

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [matchupsError, setMatchupsError] = useState(null);
  const [bracketsError, setBracketsError] = useState(null);
  const [matchupsLoading, setMatchupsLoading] = useState(false);
  const [bracketsLoading, setBracketsLoading] = useState(false);
  const [likesLoading, setLikesLoading] = useState(false);
  const [likesError, setLikesError] = useState(null);
  const [likedMatchups, setLikedMatchups] = useState([]);
  const [likedBrackets, setLikedBrackets] = useState([]);
  const [likesLoaded, setLikesLoaded] = useState(false);

  useEffect(() => {
    setLikesLoaded(false);
    setLikedMatchups([]);
    setLikedBrackets([]);
    setLikesError(null);
    setLikesLoading(false);
  }, [identifier]);

  useEffect(() => {
    let isMounted = true;

    const loadProfile = async () => {
      setLoading(true);
      setError(null);
      setMatchupsError(null);
      setBracketsError(null);
      setMatchupsLoading(true);
      setBracketsLoading(true);
      let userLookupId = identifier;

      try {
        const userRes = await getUser(identifier);
        if (!isMounted) return;
        const userData = userRes.data.response || userRes.data;
        setUser(userData);
        userLookupId = userData?.id || identifier;
      } catch (err) {
        console.error('Failed to load user', err);
        if (isMounted) {
          setError('Failed to load user data.');
          setLoading(false);
        }
        return;
      }

      try {
        const matchupsRes = await getUserMatchups(userLookupId);
        const raw = matchupsRes.data.response ?? matchupsRes.data ?? [];
        const normalized = normalizeToArray(raw);
        setMatchups(normalized.filter(isStandaloneMatchup));
      } catch (err) {
        console.warn('Matchups unavailable', err);
        setMatchups([]);
        const status = err?.response?.status;
        setMatchupsError(
          status === 403
            ? "Only followers can view this user's matchups."
            : 'Matchups unavailable.'
        );
      } finally {
        setMatchupsLoading(false);
      }

      try {
        const bracketsRes = await getUserBrackets(userLookupId);
        const raw = bracketsRes.data.response ?? bracketsRes.data ?? [];
        setBrackets(normalizeToArray(raw).filter(b => b.status !== 'archived'));
      } catch (err) {
        console.warn('Brackets unavailable', err);
        setBrackets([]);
        const status = err?.response?.status;
        setBracketsError(
          status === 403
            ? "Only followers can view this user's brackets."
            : 'Brackets unavailable.'
        );
      } finally {
        setBracketsLoading(false);
      }

      if (isMounted) setLoading(false);
    };

    loadProfile();

    return () => {
      isMounted = false;
    };
  }, [identifier]);

  useEffect(() => {
    let isMounted = true;
    const loadLikes = async () => {
      if (activeTab !== 'likes' || !user || likesLoaded) return;
      const userLookupId = user?.id || identifier;
      if (!userLookupId) return;

      setLikesLoading(true);
      setLikesError(null);

      try {
        const [matchupLikesRes, bracketLikesRes] = await Promise.all([
          getUserLikes(userLookupId),
          getUserBracketLikes(userLookupId),
        ]);

        const matchupLikes = normalizeToArray(matchupLikesRes.data);
        const bracketLikes = normalizeToArray(bracketLikesRes.data);

        const matchupIds = Array.from(
          new Set(matchupLikes.map(extractLikeMatchupId).filter(Boolean))
        );
        const bracketIds = Array.from(
          new Set(bracketLikes.map(extractLikeBracketId).filter(Boolean))
        );

        const matchupResults = await Promise.allSettled(
          matchupIds.map((id) => getMatchup(id))
        );
        const bracketResults = await Promise.allSettled(
          bracketIds.map((id) => getBracket(id))
        );

        const matchupList = matchupResults
          .filter((result) => result.status === 'fulfilled')
          .map((result) => result.value.data?.response ?? result.value.data)
          .filter(Boolean);

        const bracketList = bracketResults
          .filter((result) => result.status === 'fulfilled')
          .map((result) => result.value.data?.response ?? result.value.data)
          .filter(Boolean);

        if (!isMounted) return;
        setLikedMatchups(matchupList);
        setLikedBrackets(bracketList);
        setLikesLoaded(true);
      } catch (err) {
        console.warn('Likes unavailable', err);
        if (!isMounted) return;
        setLikesError('Likes unavailable.');
      } finally {
        if (isMounted) {
          setLikesLoading(false);
        }
      }
    };

    loadLikes();

    return () => {
      isMounted = false;
    };
  }, [activeTab, user, identifier, likesLoaded]);

  useEffect(() => {
    let isMounted = true;
    const viewerId = localStorage.getItem('userId');
    const targetId = user?.id || identifier;
    if (!viewerId || !targetId || viewerId === targetId) {
      setRelationship(null);
      return () => {
        isMounted = false;
      };
    }

    const loadRelationship = async () => {
      try {
        const res = await getUserRelationship(targetId);
        if (!isMounted) return;
        setRelationship(res.data?.response ?? res.data ?? null);
      } catch (err) {
        console.warn('Relationship unavailable', err);
        if (!isMounted) return;
        setRelationship(null);
      }
    };

    loadRelationship();

    return () => {
      isMounted = false;
    };
  }, [identifier, user?.id]);

  const normalizeFollowPayload = (raw) => {
    const payload = raw?.response ?? raw ?? {};
    const users = Array.isArray(payload.users)
      ? payload.users
      : Array.isArray(payload)
        ? payload
        : [];
    const nextCursor = payload.next_cursor ?? null;
    return { users, nextCursor };
  };

  const loadFollowers = async (reset = false) => {
    if (followersLoading && !reset) return;
    setFollowersLoading(true);
    setFollowersError(null);
    if (reset) {
      setFollowers([]);
      setFollowersCursor(null);
    }
    try {
      const params = { limit: 20 };
      const cursorValue = reset ? null : followersCursor;
      if (cursorValue) {
        params.cursor = cursorValue;
      }
      const res = await getUserFollowers(profileId, params);
      const { users, nextCursor } = normalizeFollowPayload(res.data);
      setFollowers(prev => (reset ? users : [...prev, ...users]));
      setFollowersCursor(nextCursor);
      setFollowersLoaded(true);
    } catch (err) {
      console.warn('Followers unavailable', err);
      setFollowersError('Followers unavailable.');
    } finally {
      setFollowersLoading(false);
    }
  };

  const loadFollowing = async (reset = false) => {
    if (followingLoading && !reset) return;
    setFollowingLoading(true);
    setFollowingError(null);
    if (reset) {
      setFollowing([]);
      setFollowingCursor(null);
    }
    try {
      const params = { limit: 20 };
      const cursorValue = reset ? null : followingCursor;
      if (cursorValue) {
        params.cursor = cursorValue;
      }
      const res = await getUserFollowing(profileId, params);
      const { users, nextCursor } = normalizeFollowPayload(res.data);
      setFollowing(prev => (reset ? users : [...prev, ...users]));
      setFollowingCursor(nextCursor);
      setFollowingLoaded(true);
    } catch (err) {
      console.warn('Following unavailable', err);
      setFollowingError('Following unavailable.');
    } finally {
      setFollowingLoading(false);
    }
  };

  useEffect(() => {
    setFollowers([]);
    setFollowersCursor(null);
    setFollowersLoaded(false);
    setFollowersLoading(false);
    setFollowersError(null);
    setFollowing([]);
    setFollowingCursor(null);
    setFollowingLoaded(false);
    setFollowingLoading(false);
    setFollowingError(null);
    setActiveFollowTab('followers');
    setListFollowError(null);
    loadFollowers(true);
  }, [identifier]);

  useEffect(() => {
    if (activeFollowTab !== 'following') return;
    if (!followingLoaded && !followingLoading) {
      loadFollowing(true);
    }
  }, [activeFollowTab, followingLoaded, followingLoading, identifier]);

  const handleDeleteBracket = async (bracket) => {
    if (bracket.status === 'active') {
      alert('Active brackets cannot be deleted.');
      return;
    }

    const confirmed = window.confirm('Are you sure you want to archive this bracket?');

    if (!confirmed) return;

    try {
      await archiveBracket(bracket.id);

      setBrackets(prev => prev.filter(b => b.id !== bracket.id));
    } catch (err) {
      console.error('Failed to archive bracket', err);
      alert('Failed to archive bracket.');
    }
  };

  const viewerId = localStorage.getItem('userId');
  const viewerUsername = localStorage.getItem('username');
  const profileId = user?.id || identifier;
  const profileSlug = user?.username || identifier;
  const isViewer =
    (viewerId && profileId && viewerId === profileId) ||
    (viewerUsername &&
      user?.username &&
      viewerUsername.toLowerCase() === user.username.toLowerCase());

  const matchupEmptyHeading = matchupsError ? 'Matchups unavailable' : 'No matchups yet';
  const matchupEmptyMessage = matchupsError
    ? matchupsError
    : isViewer
      ? 'You have not created a matchup yet. Share your first debate!'
      : 'No matchups to display for this user yet.';

  const bracketEmptyHeading = bracketsError ? 'Brackets unavailable' : 'No brackets yet';
  const bracketEmptyMessage = bracketsError
    ? bracketsError
    : isViewer
      ? 'You have not created a bracket yet.'
      : 'No brackets to display for this user yet.';

  const standaloneMatchups = matchups.filter(isStandaloneMatchup);
  const matchupCount = standaloneMatchups.length;
  const bracketCount = brackets.length;

  const handleFollowToggle = async () => {
    setFollowError(null);
    if (!viewerId) {
      navigate('/login');
      return;
    }
    if (isViewer || followBusy) return;

    const isFollowing = Boolean(relationship?.following);
    setFollowBusy(true);
    try {
      if (isFollowing) {
        await unfollowUser(profileId);
        setRelationship(prev => ({
          following: false,
          followed_by: Boolean(prev?.followed_by),
          mutual: false,
        }));
        setUser(prev =>
          prev
            ? {
                ...prev,
                followers_count: Math.max((prev.followers_count || 0) - 1, 0),
              }
            : prev
        );
      } else {
        await followUser(profileId);
        setRelationship(prev => {
          const followedBy = Boolean(prev?.followed_by);
          return {
            following: true,
            followed_by: followedBy,
            mutual: followedBy,
          };
        });
        setUser(prev =>
          prev
            ? {
                ...prev,
                followers_count: (prev.followers_count || 0) + 1,
              }
            : prev
        );
      }
    } catch (err) {
      console.error('Follow action failed', err);
      setFollowError('Unable to update follow status.');
    } finally {
      setFollowBusy(false);
    }
  };

  const handlePrivacyToggle = async () => {
    if (!isViewer || privacyUpdating || !user) return;
    setPrivacyError(null);
    setPrivacyUpdating(true);
    const nextValue = !user.is_private;
    try {
      const res = await updateUserPrivacy(profileId, nextValue);
      const payload = res.data?.response ?? res.data ?? null;
      if (payload) {
        setUser(prev => (prev ? { ...prev, ...payload } : payload));
      } else {
        setUser(prev => (prev ? { ...prev, is_private: nextValue } : prev));
      }
    } catch (err) {
      console.error('Privacy update failed', err);
      setPrivacyError('Unable to update privacy settings.');
    } finally {
      setPrivacyUpdating(false);
    }
  };

  const refreshUserCounts = async () => {
    try {
      const res = await getUser(profileId);
      const payload = res.data?.response ?? res.data ?? null;
      if (!payload) return;
      setUser(prev => (prev ? { ...prev, ...payload } : payload));
    } catch (err) {
      console.warn('Failed to refresh user counts', err);
    }
  };

  const updateUserLists = (targetId, updater) => {
    setFollowers(prev => prev.map(person => (person.id === targetId ? updater(person) : person)));
    setFollowing(prev => prev.map(person => (person.id === targetId ? updater(person) : person)));
  };

  const handleListFollowToggle = async (person) => {
    setListFollowError(null);
    if (!viewerId) {
      navigate('/login');
      return;
    }
    if (listFollowBusyId) return;

    const isFollowing = Boolean(person.viewer_following);
    setListFollowBusyId(person.id);
    try {
      if (isFollowing) {
        await unfollowUser(person.id);
        updateUserLists(person.id, (userEntry) => ({
          ...userEntry,
          viewer_following: false,
          mutual: false,
          followers_count: Math.max((userEntry.followers_count || 0) - 1, 0),
        }));
      } else {
        await followUser(person.id);
        updateUserLists(person.id, (userEntry) => {
          const followedBy = Boolean(userEntry.viewer_followed_by);
          return {
            ...userEntry,
            viewer_following: true,
            mutual: followedBy,
            followers_count: (userEntry.followers_count || 0) + 1,
          };
        });
      }
    } catch (err) {
      console.error('Follow action failed', err);
      setListFollowError('Unable to update follow status.');
    } finally {
      setListFollowBusyId(null);
    }
  };

  const openFollowModal = (tab) => {
    setActiveFollowTab(tab);
    setFollowModalOpen(true);
  };

  const bio = user?.bio ?? user?.Bio ?? '';
  const displayBio = bio || (isViewer ? 'Add a short bio to introduce yourself.' : 'No bio yet.');

  const progressBadges = useMemo(() => {
    const badges = [];
    if (isViewer && !user?.avatar_path) {
      badges.push('Complete your profile');
    }
    if (matchupCount === 0) {
      badges.push('Create your first matchup');
    }
    if (bracketCount === 0) {
      badges.push('Start your first bracket');
    }
    return badges;
  }, [isViewer, user?.avatar_path, matchupCount, bracketCount]);

  const profileStats = [
    {
      label: 'Followers',
      value: formatStat(user?.followers_count ?? 0),
      onClick: () => openFollowModal('followers'),
    },
    {
      label: 'Following',
      value: formatStat(user?.following_count ?? 0),
      onClick: () => openFollowModal('following'),
    },
    { label: 'Matchups', value: formatStat(matchupCount) },
    { label: 'Brackets', value: formatStat(bracketCount) },
  ];

  return (
    <div className="profile-page">
      <NavigationBar />

      <main className="profile-content">
        {loading && (
          <div className="profile-status-card">Loading profile...</div>
        )}

        {error && !loading && (
          <div className="profile-status-card profile-status-card--error">
            {error}
          </div>
        )}

        {!loading && !error && user && (
          <>
            <section className="profile-hero">
              <div className="profile-hero-main">
                <ProfilePic userId={profileId} editable={isViewer} size={96} />
                <div className="profile-hero-info">
                  <div className="profile-hero-title-row">
                    <h1>{user.username || user.name || 'Matchup Fan'}</h1>
                    {!isViewer && (
                      <div className="profile-follow-row">
                        {relationship?.followed_by && (
                          <span className="profile-follow-badge">Follows you</span>
                        )}
                        {relationship?.mutual && (
                          <span className="profile-follow-badge profile-follow-badge--mutual">
                            Mutual
                          </span>
                        )}
                      </div>
                    )}
                  </div>
                  <p className="profile-bio">{displayBio}</p>
                  {progressBadges.length > 0 && (
                    <div className="profile-progress-row">
                      {progressBadges.map((badge) => (
                        <span key={badge} className="profile-progress-badge">
                          {badge}
                        </span>
                      ))}
                    </div>
                  )}
                  <div className="profile-stats-row">
                    {profileStats.map((stat) => {
                      const StatTag = stat.onClick ? 'button' : 'div';
                      return (
                        <StatTag
                          key={stat.label}
                          type={stat.onClick ? 'button' : undefined}
                          className={`profile-stat ${stat.onClick ? 'profile-stat--clickable' : ''}`}
                          onClick={stat.onClick}
                        >
                          <span>{stat.label}</span>
                          <strong>{stat.value}</strong>
                        </StatTag>
                      );
                    })}
                  </div>
                  {followError && (
                    <div className="profile-follow-error">{followError}</div>
                  )}
                  <div className="profile-hero-actions">
                    {!isViewer && (
                      <>
                        <Button
                          onClick={handleFollowToggle}
                          className={relationship?.following ? 'profile-secondary-button' : 'profile-primary-button'}
                          disabled={followBusy}
                        >
                          {!viewerId
                            ? 'Log in to follow'
                            : relationship?.following
                              ? 'Following'
                              : 'Follow'}
                        </Button>
                        <Button
                          className="profile-secondary-button profile-secondary-button--disabled"
                          disabled
                        >
                          <FiMessageCircle /> Message
                        </Button>
                      </>
                    )}
                    {isViewer && (
                      <Button
                        className="profile-secondary-button"
                        disabled
                        title="Profile editing coming soon"
                      >
                        Edit profile
                      </Button>
                    )}
                  </div>

                  {isViewer && (
                    <div className="profile-privacy-row">
                      <span className="profile-privacy-label">Profile visibility</span>
                      <span
                        className={`profile-privacy-pill ${
                          user.is_private ? 'profile-privacy-pill--private' : 'profile-privacy-pill--public'
                        }`}
                      >
                        {user.is_private ? 'Followers only' : 'Public'}
                      </span>
                      <Button
                        onClick={handlePrivacyToggle}
                        className="profile-secondary-button"
                        disabled={privacyUpdating}
                      >
                        {user.is_private ? 'Make public' : 'Make followers-only'}
                      </Button>
                    </div>
                  )}
                  {privacyError && (
                    <div className="profile-privacy-error">{privacyError}</div>
                  )}
                </div>
              </div>
            </section>

            <Tabs.Root value={activeTab} onValueChange={setActiveTab} className="profile-tabs-root">
              <Tabs.List className="profile-tabs">
                <Tabs.Trigger value="matchups" className="profile-tab">
                  Matchups
                </Tabs.Trigger>
                <Tabs.Trigger value="brackets" className="profile-tab">
                  Brackets
                </Tabs.Trigger>
                <Tabs.Trigger value="activity" className="profile-tab">
                  Activity
                </Tabs.Trigger>
                <Tabs.Trigger value="likes" className="profile-tab">
                  Likes
                </Tabs.Trigger>
              </Tabs.List>

              <Tabs.Content value="matchups" className="profile-tab-panel">
                <div className="profile-tab-section">
                  <header className="profile-section-header">
                    <div>
                      <h2>{isViewer ? 'Your Matchups' : `${user.username}'s Matchups`}</h2>
                      <p>Votes happening now across your latest debates.</p>
                    </div>
                    {isViewer && (
                      <Button
                        onClick={() => navigate(`/users/${profileSlug}/create-matchup`)}
                        className="profile-secondary-button"
                      >
                        New matchup
                      </Button>
                    )}
                  </header>

                  <div className="profile-grid profile-grid--compact">
                    {matchupsLoading && (
                      Array.from({ length: 3 }).map((_, index) => (
                        <SkeletonCard key={`matchup-skeleton-${index}`} />
                      ))
                    )}
                    {!matchupsLoading && standaloneMatchups.length > 0 && (
                      <AnimatePresence initial={false}>
                        {standaloneMatchups.map(m => (
                          <motion.article
                            key={m.id}
                            className="profile-card profile-card--compact"
                            layout
                            initial={{ opacity: 0, y: 12 }}
                            animate={{ opacity: 1, y: 0 }}
                            exit={{ opacity: 0, y: -10 }}
                            transition={{ duration: 0.25 }}
                            whileHover={{ y: -4 }}
                          >
                            <div>
                              <h3>{m.title}</h3>
                              <p>{(m.content || '').slice(0, 120) || 'No description yet.'}</p>
                            </div>
                            <Link to={`/users/${profileSlug}/matchup/${m.id}`} className="profile-card-link">
                              View matchup ->
                            </Link>
                          </motion.article>
                        ))}
                      </AnimatePresence>
                    )}
                    {!matchupsLoading && standaloneMatchups.length === 0 && (
                      <EmptyStateCard
                        title={matchupEmptyHeading}
                        description={matchupEmptyMessage}
                        ctaLabel={isViewer ? 'Start your first matchup' : 'Browse matchups'}
                        onCta={() => (isViewer ? navigate(`/users/${profileSlug}/create-matchup`) : navigate('/home'))}
                        suggestions={['Drake vs Lil Wayne', 'iPhone vs Android', 'Best Beyonce album']}
                        tips={['Try a template', 'Invite two friends to vote']}
                        icon={FiZap}
                      />
                    )}
                  </div>
                </div>
              </Tabs.Content>

              <Tabs.Content value="brackets" className="profile-tab-panel">
                <div className="profile-tab-section">
                  <header className="profile-section-header">
                    <div>
                      <h2>{isViewer ? 'Your Brackets' : `${user.username}'s Brackets`}</h2>
                      <p>Active brackets and tournament history.</p>
                    </div>
                    {isViewer && (
                      <Button
                        onClick={() => navigate('/brackets/new')}
                        className="profile-secondary-button"
                      >
                        New bracket
                      </Button>
                    )}
                  </header>

                  <div className="profile-grid">
                    {bracketsLoading && (
                      Array.from({ length: 3 }).map((_, index) => (
                        <SkeletonCard key={`bracket-skeleton-${index}`} />
                      ))
                    )}
                    {!bracketsLoading && brackets.length > 0 && (
                      <AnimatePresence initial={false}>
                        {brackets.map(b => (
                          <motion.article
                            key={b.id}
                            className="profile-card"
                            layout
                            initial={{ opacity: 0, y: 12 }}
                            animate={{ opacity: 1, y: 0 }}
                            exit={{ opacity: 0, y: -10 }}
                            transition={{ duration: 0.25 }}
                            whileHover={{ y: -4 }}
                          >
                            <div>
                              <h3>{b.title}</h3>
                              <p>{b.description || 'No description yet.'}</p>
                              <p className="profile-card-meta">Status: {b.status || 'draft'}</p>
                            </div>
                            <div className="profile-card-actions">
                              <Link to={`/brackets/${b.id}`} className="profile-card-link">
                                View bracket ->
                              </Link>
                              {isViewer && (
                                <button
                                  type="button"
                                  className="profile-card-muted"
                                  onClick={() => handleDeleteBracket(b)}
                                >
                                  Archive
                                </button>
                              )}
                            </div>
                          </motion.article>
                        ))}
                      </AnimatePresence>
                    )}
                    {!bracketsLoading && brackets.length === 0 && (
                      <EmptyStateCard
                        title={bracketEmptyHeading}
                        description={bracketEmptyMessage}
                        ctaLabel={isViewer ? 'Start a bracket' : 'Browse brackets'}
                        onCta={() => (isViewer ? navigate('/brackets/new') : navigate('/home'))}
                        suggestions={['Best 90s R&B', 'NBA goat bracket', 'Top anime openings']}
                        tips={['Seed your contenders', 'Share after round one']}
                        icon={FiStar}
                      />
                    )}
                  </div>
                </div>
              </Tabs.Content>

              <Tabs.Content value="activity" className="profile-tab-panel">
                <div className="profile-tab-section">
                  <header className="profile-section-header">
                    <div>
                      <h2>Activity</h2>
                      <p>A timeline of votes, wins, and new debates.</p>
                    </div>
                  </header>
                  <EmptyStateCard
                    title="Your activity timeline is quiet"
                    description="Follow a few creators or jump into a matchup to start seeing activity here."
                    ctaLabel="Browse trending"
                    onCta={() => navigate('/home')}
                    suggestions={['Vote on a matchup', 'Follow 3 creators', 'Share your first matchup']}
                    tips={['Votes happening now', 'Trending matchups refresh daily']}
                    icon={FiActivity}
                  />
                </div>
              </Tabs.Content>

              <Tabs.Content value="likes" className="profile-tab-panel">
                <div className="profile-tab-section">
                  <header className="profile-section-header">
                    <div>
                      <h2>Likes</h2>
                      <p>Matchups and brackets you have bookmarked.</p>
                    </div>
                  </header>
                  {likesLoading && (
                    <div className="profile-grid profile-grid--compact">
                      {Array.from({ length: 3 }).map((_, index) => (
                        <SkeletonCard key={`likes-skeleton-${index}`} />
                      ))}
                    </div>
                  )}

                  {!likesLoading && likesError && (
                    <EmptyStateCard
                      title="Likes unavailable"
                      description={likesError}
                      ctaLabel="Browse trending"
                      onCta={() => navigate('/home')}
                      tips={['Trending now', 'Votes happening now']}
                      icon={FiTrendingUp}
                    />
                  )}

                  {!likesLoading && !likesError && likedMatchups.length === 0 && likedBrackets.length === 0 && (
                    <EmptyStateCard
                      title="No likes yet"
                      description="Start liking matchups to build your personal collection."
                      ctaLabel="Discover matchups"
                      onCta={() => navigate('/home')}
                      suggestions={['Find a trending matchup', 'Join a bracket', 'Vote in a debate']}
                      tips={['Trending now', 'Votes happening now']}
                      icon={FiTrendingUp}
                    />
                  )}

                  {!likesLoading && !likesError && (likedMatchups.length > 0 || likedBrackets.length > 0) && (
                    <div className="profile-likes-grid">
                      {likedMatchups.length > 0 && (
                        <div className="profile-tab-section">
                          <header className="profile-section-header profile-section-header--compact">
                            <div>
                              <h3>Matchups you liked</h3>
                            </div>
                          </header>
                          <div className="profile-grid profile-grid--compact">
                            {likedMatchups.map((matchup) => (
                              <motion.article
                                key={`liked-matchup-${matchup.id}`}
                                className="profile-card profile-card--compact"
                                layout
                                initial={{ opacity: 0, y: 12 }}
                                animate={{ opacity: 1, y: 0 }}
                                exit={{ opacity: 0, y: -10 }}
                                transition={{ duration: 0.25 }}
                                whileHover={{ y: -4 }}
                              >
                                <div>
                                  <h3>{matchup.title}</h3>
                                  <p>{(matchup.content || '').slice(0, 120) || 'No description yet.'}</p>
                                </div>
                                <Link
                                  to={`/users/${matchup.author?.username || matchup.author_username || matchup.author_id}/matchup/${matchup.id}`}
                                  className="profile-card-link"
                                >
                                  View matchup ->
                                </Link>
                              </motion.article>
                            ))}
                          </div>
                        </div>
                      )}

                      {likedBrackets.length > 0 && (
                        <div className="profile-tab-section">
                          <header className="profile-section-header profile-section-header--compact">
                            <div>
                              <h3>Brackets you liked</h3>
                            </div>
                          </header>
                          <div className="profile-grid">
                            {likedBrackets.map((bracket) => (
                              <motion.article
                                key={`liked-bracket-${bracket.id}`}
                                className="profile-card"
                                layout
                                initial={{ opacity: 0, y: 12 }}
                                animate={{ opacity: 1, y: 0 }}
                                exit={{ opacity: 0, y: -10 }}
                                transition={{ duration: 0.25 }}
                                whileHover={{ y: -4 }}
                              >
                                <div>
                                  <h3>{bracket.title}</h3>
                                  <p>{bracket.description || 'No description yet.'}</p>
                                  <p className="profile-card-meta">Status: {bracket.status || 'draft'}</p>
                                </div>
                                <Link to={`/brackets/${bracket.id}`} className="profile-card-link">
                                  View bracket ->
                                </Link>
                              </motion.article>
                            ))}
                          </div>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              </Tabs.Content>
            </Tabs.Root>
          </>
        )}
      </main>

      <FollowListModal
        isOpen={followModalOpen}
        onClose={() => {
          setFollowModalOpen(false);
          refreshUserCounts();
        }}
        activeTab={activeFollowTab}
        onTabChange={setActiveFollowTab}
        followers={followers}
        following={following}
        followersLoading={followersLoading}
        followingLoading={followingLoading}
        followersError={followersError}
        followingError={followingError}
        followersCursor={followersCursor}
        followingCursor={followingCursor}
        onLoadMoreFollowers={loadFollowers}
        onLoadMoreFollowing={loadFollowing}
        onFollowToggle={handleListFollowToggle}
        listFollowBusyId={listFollowBusyId}
        viewerId={viewerId}
        resolveAvatarUrl={resolveAvatarUrl}
        listFollowError={listFollowError}
      />
    </div>
  );
};

export default UserProfile;
