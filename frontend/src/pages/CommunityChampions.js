import React, { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { getCommunityBySlug, getCommunityChampions } from '../services/api';
import { relativeTime } from '../utils/time';
import '../styles/CommunityChampions.css';

/*
 * CommunityChampions — `/c/<slug>/champions` leaderboard.
 *
 * Reads community_member_wins via GetCommunityChampions and renders
 * a ranked list. Each row: rank + avatar + @username + wins +
 * relative "last won 3d ago" timestamp.
 *
 * Why a dedicated page (instead of a tab inside CommunityPage):
 *   - Champions is read-heavy + the data is its own query. Splitting
 *     keeps CommunityPage's mount surface small.
 *   - Direct routing means Champions has a shareable URL —
 *     /c/cordell/champions can be bookmarked / linked from
 *     external posts ("look who's winning my community").
 *   - The community context (theme gradient, name) is fetched
 *     locally so the page can render without depending on
 *     CommunityPage state.
 *
 * Empty-state: a friendly "No champions yet" panel that points at
 * how wins are earned (be added as a contender, win the matchup).
 */
const CommunityChampions = () => {
  const { slug } = useParams();
  const [community, setCommunity] = useState(null);
  const [champions, setChampions] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    (async () => {
      try {
        const cRes = await getCommunityBySlug(slug);
        if (cancelled) return;
        const c = cRes?.data?.community ?? null;
        setCommunity(c);
        if (c?.id) {
          const champRes = await getCommunityChampions(c.id, { limit: 50 });
          if (!cancelled) setChampions(champRes?.data?.champions || []);
        }
      } catch (err) {
        if (!cancelled) {
          const status = err?.response?.status;
          setError(status === 404 ? 'Community not found.' : 'Could not load champions.');
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [slug]);

  if (loading) {
    return (
      <div className="community-champions community-champions--loading">
        <div className="community-champions__spinner" aria-label="Loading champions" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="community-champions community-champions--error">
        <h1>{error}</h1>
        <Link to={`/c/${slug}`}>← Back to community</Link>
      </div>
    );
  }

  return (
    <div className="community-champions">
      <header className="community-champions__header">
        <p className="community-champions__overline">
          <Link to={`/c/${slug}`}>/c/{slug}</Link>
        </p>
        <h1 className="community-champions__title">Champions</h1>
        <p className="community-champions__subtitle">
          Top {community?.name || 'community'} members by matchup wins.
        </p>
      </header>

      {champions.length === 0 ? (
        <div className="community-champions__empty">
          <h2>No champions yet</h2>
          <p>
            Wins accrue when a member is added as a matchup contender
            and their item gets the most votes. Start a community matchup
            with someone as a contender to seed the leaderboard.
          </p>
        </div>
      ) : (
        <ol className="community-champions__list">
          {champions.map((c, idx) => (
            <li key={c.user_id} className="community-champions__row">
              <span className="community-champions__rank">{idx + 1}</span>
              <Link to={`/users/${c.username}`} className="community-champions__profile">
                {c.avatar_path ? (
                  <img src={c.avatar_path} alt="" className="community-champions__avatar" />
                ) : (
                  <span className="community-champions__avatar community-champions__avatar--initial">
                    {(c.username || '?').charAt(0).toUpperCase()}
                  </span>
                )}
                <span className="community-champions__username">@{c.username}</span>
              </Link>
              <span className="community-champions__wins">
                <strong>{c.wins_count}</strong>{' '}
                {c.wins_count === 1 ? 'win' : 'wins'}
              </span>
              {c.last_won_at && (
                <span className="community-champions__last-won">
                  Last won {relativeTime(c.last_won_at)}
                </span>
              )}
            </li>
          ))}
        </ol>
      )}
    </div>
  );
};

export default CommunityChampions;
