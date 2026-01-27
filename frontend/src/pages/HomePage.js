// frontend/src/pages/HomePage.js
import React, { useState, useEffect, useMemo, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { AnimatePresence, motion } from 'framer-motion';
import {
  FiTrendingUp,
  FiZap,
  FiUsers,
  FiStar,
  FiArrowRight,
  FiActivity,
} from 'react-icons/fi';
import { getHomeSummary } from '../services/api';
import NavigationBar from '../components/NavigationBar';
import Button from '../components/Button';
import EmptyStateCard from '../components/EmptyStateCard';
import SkeletonCard from '../components/SkeletonCard';
import Reveal from '../components/Reveal';
import StatSparkline from '../components/StatSparkline';
import '../styles/HomePage.css';

const demoTrending = [
  { title: 'Drake vs Lil Wayne', subtitle: 'Best rapper debate', tag: 'Music' },
  { title: 'iPhone vs Android', subtitle: 'The forever showdown', tag: 'Tech' },
  { title: 'Marvel vs DC', subtitle: 'Who owns the multiverse?', tag: 'Movies' },
  { title: 'Summer Walker vs SZA', subtitle: 'Vibes only', tag: 'R&B' },
  { title: 'Best Anita Baker album', subtitle: 'Classic soul picks', tag: 'Classics' },
];

const creatorFallbacks = [
  { name: 'House of Debates', tagline: 'Pop culture hot takes' },
  { name: 'Bracket Theory', tagline: 'Tournament specialists' },
  { name: 'Late Night Voting', tagline: 'The midnight matchups' },
];

const statValue = (value) => {
  if (value === null || value === undefined) return '--';
  if (typeof value === 'number' && value === 0) return '--';
  return value;
};

const HomePage = () => {
  const [matchups, setMatchups] = useState([]);
  const [brackets, setBrackets] = useState([]);
  const [votesToday, setVotesToday] = useState(null);
  const [activeMatchups, setActiveMatchups] = useState(null);
  const [activeBrackets, setActiveBrackets] = useState(null);
  const [topCreator, setTopCreator] = useState(null);
  const [creatorsToFollow, setCreatorsToFollow] = useState([]);
  const [newThisWeek, setNewThisWeek] = useState([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(null);
  const [bracketsLoading, setBracketsLoading] = useState(true);
  const [bracketsError, setBracketsError] = useState(null);

  const navigate = useNavigate();
  const userId = localStorage.getItem('userId');
  const username = localStorage.getItem('username');
  const trendingRef = useRef(null);

  const navigateToCreateMatchup = () => {
    const storedUserId = localStorage.getItem('userId');
    const storedUsername = localStorage.getItem('username');
    if (!storedUserId) {
      navigate('/login');
      return;
    }
    navigate(`/users/${storedUsername || storedUserId}/create-matchup`);
  };

  const navigateToCreateBracket = () => {
    if (!userId) {
      navigate('/login');
      return;
    }
    navigate('/brackets/new');
  };

  const handleBrowseTrending = () => {
    if (trendingRef.current) {
      trendingRef.current.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
  };

  useEffect(() => {
    const fetchHomeSummary = async () => {
      setIsLoading(true);
      setBracketsLoading(true);

      try {
        const response = await getHomeSummary(userId);
        const data = response.data?.response ?? response.data ?? {};
        const matchupsData = data.popular_matchups ?? [];
        const bracketsData = data.popular_brackets ?? [];

        setMatchups(Array.isArray(matchupsData) ? matchupsData : []);
        setBrackets(Array.isArray(bracketsData) ? bracketsData : []);
        setVotesToday(
          typeof data.votes_today === 'number'
            ? data.votes_today
            : typeof data.votesToday === 'number'
              ? data.votesToday
              : null
        );
        setActiveMatchups(
          typeof data.active_matchups === 'number'
            ? data.active_matchups
            : typeof data.activeMatchups === 'number'
              ? data.activeMatchups
              : null
        );
        setActiveBrackets(
          typeof data.active_brackets === 'number'
            ? data.active_brackets
            : typeof data.activeBrackets === 'number'
              ? data.activeBrackets
              : null
        );
        setTopCreator(data.top_creator ?? data.topCreator ?? null);
        setCreatorsToFollow(
          Array.isArray(data.creators_to_follow)
            ? data.creators_to_follow
            : Array.isArray(data.creatorsToFollow)
              ? data.creatorsToFollow
              : []
        );
        setNewThisWeek(
          Array.isArray(data.new_this_week)
            ? data.new_this_week
            : Array.isArray(data.newThisWeek)
              ? data.newThisWeek
              : []
        );
        setError(null);
        setBracketsError(null);
      } catch (err) {
        console.error('Failed to fetch home summary:', err);
        setError('Popular matchups are not available right now.');
        setBracketsError('Popular brackets are not available right now.');
        setActiveMatchups(null);
        setActiveBrackets(null);
        setTopCreator(null);
        setCreatorsToFollow([]);
        setNewThisWeek([]);
      } finally {
        setIsLoading(false);
        setBracketsLoading(false);
      }
    };

    fetchHomeSummary();
  }, [userId]);


  const hasMatchups = matchups.length > 0;
  const hasBrackets = brackets.length > 0;

  const trendingCards = useMemo(() => {
    if (hasMatchups) {
      return matchups.map((matchup) => {
        const matchupId = matchup.id ?? matchup.matchup_id ?? matchup.matchupId;
        const matchupTitle = matchup.title || `Matchup #${matchupId}`;
        const ownerId = matchup.author_id ?? matchup.user_id ?? matchup.owner_id ?? null;
        return {
          id: matchupId,
          title: matchupTitle,
          subtitle: `${Math.round(matchup.engagement_score ?? 0)} engagements`,
          tag: 'Trending now',
          link: ownerId ? `/users/${ownerId}/matchup/${matchupId}` : null,
        };
      });
    }

    return demoTrending.map((item, index) => ({
      id: `demo-${index}`,
      title: item.title,
      subtitle: item.subtitle,
      tag: item.tag,
      link: null,
    }));
  }, [hasMatchups, matchups]);

  const creatorSuggestions = useMemo(() => {
    if (creatorsToFollow.length > 0) {
      return creatorsToFollow.slice(0, 3).map((creator) => ({
        id: creator.id,
        name: creator.username || `Creator #${creator.id}`,
        tagline:
          typeof creator.followers_count === 'number'
            ? `${creator.followers_count} followers`
            : typeof creator.followersCount === 'number'
              ? `${creator.followersCount} followers`
            : 'Curated debates',
      }));
    }
    return creatorFallbacks.map((creator, index) => ({
      id: null,
      name: creator.name,
      tagline: creator.tagline,
      key: `fallback-${index}`,
    }));
  }, [creatorsToFollow]);

  const topCreatorName =
    topCreator?.username ||
    topCreator?.name ||
    (topCreator?.id ? `Creator #${topCreator.id}` : '--');

  const buildTrendSeries = (value) => {
    const base = Number.isFinite(Number(value)) ? Number(value) : 0;
    const multipliers = [0.72, 0.86, 0.95, 1.05, 0.98, 1.12, 1.0];
    return multipliers.map((mult, index) => ({
      index,
      value: Math.max(0, Math.round(base * mult)),
    }));
  };

  const statTrends = useMemo(
    () => ({
      votes: buildTrendSeries(votesToday),
      matchups: buildTrendSeries(activeMatchups),
      brackets: buildTrendSeries(activeBrackets),
    }),
    [votesToday, activeMatchups, activeBrackets],
  );

  const stats = [
    {
      label: 'Votes happening now',
      value: statValue(votesToday),
      icon: FiActivity,
      trend: statTrends.votes,
      color: '#f59e0b',
    },
    {
      label: 'Active matchups',
      value: statValue(activeMatchups),
      icon: FiZap,
      trend: statTrends.matchups,
      color: '#38bdf8',
    },
    {
      label: 'Active brackets',
      value: statValue(activeBrackets),
      icon: FiStar,
      trend: statTrends.brackets,
      color: '#a78bfa',
    },
    { label: 'Top creator', value: topCreatorName, icon: FiUsers },
  ];

  return (
    <div className="home-page">
      <NavigationBar />

      <main className="home-content">
        <Reveal as="section" className="home-hero">
          <div className="home-hero-copy">
            <p className="home-hero-kicker">Browse first, create second.</p>
            <h1>Settle debates with friends.</h1>
            <p className="home-hero-subtitle">
              Make matchups. Vote. Share. See what wins.
            </p>
            <div className="home-hero-actions">
              <Button onClick={navigateToCreateMatchup} className="home-primary-button">
                Create matchup
              </Button>
              <Button
                onClick={handleBrowseTrending}
                className="home-secondary-button"
              >
                Browse trending
              </Button>
            </div>
          </div>
          <div className="home-hero-panel">
            <div className="home-hero-panel-card">
              <p className="home-hero-panel-title">Trending now</p>
              <div className="home-hero-panel-list">
                {trendingCards.slice(0, 3).map((item) => (
                  <div key={item.id} className="home-hero-panel-item">
                    <span>{item.title}</span>
                    <small>{item.subtitle}</small>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </Reveal>

        <Reveal as="section" className="home-stats-row">
          {stats.map((stat) => {
            const Icon = stat.icon;
            return (
              <div key={stat.label} className="home-stat-card">
                <div className="home-stat-header">
                  <Icon aria-hidden="true" />
                  <span>{stat.label}</span>
                </div>
                <strong>{stat.value}</strong>
                {stat.trend && (
                  <StatSparkline data={stat.trend} color={stat.color} />
                )}
              </div>
            );
          })}
        </Reveal>

        <Reveal as="section" className="home-trending-strip" ref={trendingRef}>
          <div className="home-section-header">
            <div>
              <h2>Trending now</h2>
              <p>Votes happening now across the community.</p>
            </div>
          </div>
          <div className="home-trending-scroll">
            {isLoading
              ? Array.from({ length: 4 }).map((_, index) => (
                  <motion.div
                    key={`trend-skeleton-${index}`}
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    className="rounded-2xl border border-slate-700/40 bg-slate-900/60 p-6"
                  >
                    <div className="h-4 w-20 rounded-full bg-slate-700/50 animate-pulse" />
                    <div className="mt-4 space-y-3">
                      <div className="h-4 w-3/4 rounded-full bg-slate-700/50 animate-pulse" />
                      <div className="h-3 w-full rounded-full bg-slate-700/40 animate-pulse" />
                    </div>
                  </motion.div>
                ))
              : trendingCards.map((item) => (
                  <motion.article
                    key={item.id}
                    className="home-trending-card"
                    whileHover={{ y: -4 }}
                  >
                    <span className="home-trending-tag">{item.tag}</span>
                    <h3>{item.title}</h3>
                    <p>{item.subtitle}</p>
                    {item.link ? (
                      <Button
                        onClick={() => navigate(item.link)}
                        className="home-trending-link"
                      >
                        View matchup <FiArrowRight />
                      </Button>
                    ) : (
                      <span className="home-trending-link home-trending-link--muted">
                        Demo only
                      </span>
                    )}
                  </motion.article>
                ))}
          </div>
        </Reveal>

        <Reveal as="section" className="home-grid">
          <div className="home-main">
            <div className="home-section-block">
              <header className="home-section-header">
                <div>
                  <h2>Trending matchups</h2>
                  <p>See what the community is engaging with right now.</p>
                </div>
              </header>

              {isLoading && (
                <div className="home-card-grid">
                  {Array.from({ length: 3 }).map((_, index) => (
                    <SkeletonCard key={`matchup-skeleton-${index}`} />
                  ))}
                </div>
              )}

              {!isLoading && (!hasMatchups || error) && (
                <EmptyStateCard
                  title="Be the first to spark a debate"
                  description={
                    error
                      ? error
                      : "Create a matchup in 20 seconds - we'll help you pick a template."
                  }
                  ctaLabel="Create your first matchup"
                  onCta={navigateToCreateMatchup}
                  suggestions={[
                    'Drake vs Lil Wayne',
                    'iPhone vs Android',
                    'Best Beyonce album',
                  ]}
                  tips={['Invite two friends', 'Share the link in a group chat']}
                  icon={FiZap}
                />
              )}

              {!isLoading && hasMatchups && !error && (
                <div className="home-card-grid">
                  <AnimatePresence initial={false}>
                    {matchups.map((matchup) => {
                      const matchupId = matchup.id ?? matchup.matchup_id ?? matchup.matchupId;
                      const matchupTitle = matchup.title || `Matchup #${matchupId}`;
                      const matchupOwnerId =
                        matchup.author_id ?? matchup.user_id ?? matchup.owner_id ?? userId;

                      return (
                        <motion.article
                          key={matchupId}
                          className="home-card"
                          layout
                          initial={{ opacity: 0, y: 12 }}
                          animate={{ opacity: 1, y: 0 }}
                          exit={{ opacity: 0, y: -10 }}
                          transition={{ duration: 0.25 }}
                          whileHover={{ y: -4 }}
                        >
                          <div>
                            <h3>{matchupTitle}</h3>
                            <p>
                              {typeof matchup.rank !== 'undefined' && `Rank #${matchup.rank}`}
                              {typeof matchup.engagement_score !== 'undefined' &&
                                ` | ${Math.round(matchup.engagement_score)} engagements`}
                            </p>
                          </div>
                          {matchupOwnerId ? (
                            <Button
                              onClick={() =>
                                navigate(`/users/${matchupOwnerId}/matchup/${matchupId}`)
                              }
                              className="home-card-link"
                            >
                              View matchup
                            </Button>
                          ) : (
                            <span className="home-card-link home-card-link--muted">Owner missing</span>
                          )}
                        </motion.article>
                      );
                    })}
                  </AnimatePresence>
                </div>
              )}
            </div>

            <div className="home-section-block">
              <header className="home-section-header">
                <div>
                  <h2>Active brackets</h2>
                  <p>Ongoing tournaments the community is watching.</p>
                </div>
                <Button
                  onClick={navigateToCreateBracket}
                  className="home-outline-button"
                >
                  Create bracket
                </Button>
              </header>

              {bracketsLoading && (
                <div className="home-card-grid">
                  {Array.from({ length: 3 }).map((_, index) => (
                    <SkeletonCard key={`bracket-skeleton-${index}`} />
                  ))}
                </div>
              )}

              {!bracketsLoading && (!hasBrackets || bracketsError) && (
                <EmptyStateCard
                  title="Build the next tournament"
                  description={
                    bracketsError
                      ? bracketsError
                      : 'Start a bracket and invite others to vote on every round.'
                  }
                  ctaLabel="Create a bracket"
                  onCta={navigateToCreateBracket}
                  suggestions={['Best 90s R&B', 'NBA goat bracket', 'Top anime openings']}
                  tips={['Seed your contenders', 'Share after round one']}
                  icon={FiStar}
                />
              )}

              {!bracketsLoading && hasBrackets && !bracketsError && (
                <div className="home-card-grid">
                  {brackets.map((bracket) => {
                    const bracketId = bracket.id ?? bracket.bracket_id ?? bracket.bracketId;
                    const bracketTitle = bracket.title || `Bracket #${bracketId}`;

                    return (
                      <motion.article
                        key={bracketId}
                        className="home-card"
                        whileHover={{ y: -4 }}
                      >
                        <div>
                          <h3>{bracketTitle}</h3>
                          <p>
                            {typeof bracket.rank !== 'undefined' && `Rank #${bracket.rank}`}
                            {typeof bracket.current_round !== 'undefined' &&
                              ` | Round ${bracket.current_round}`}
                          </p>
                        </div>
                        {bracketId ? (
                          <Button
                            onClick={() => navigate(`/brackets/${bracketId}`)}
                            className="home-card-link"
                          >
                            View bracket
                          </Button>
                        ) : (
                          <span className="home-card-link home-card-link--muted">Bracket missing</span>
                        )}
                      </motion.article>
                    );
                  })}
                </div>
              )}
            </div>

            <div className="home-section-block">
              <header className="home-section-header">
                <div>
                  <h2>New this week</h2>
                  <p>Fresh matchups to jump into while they are hot.</p>
                </div>
              </header>

              {newThisWeek.length === 0 ? (
                <EmptyStateCard
                  title="Fresh debates are coming"
                  description="Kick off a matchup and we will feature it here all week."
                  ctaLabel="Start a matchup"
                  onCta={navigateToCreateMatchup}
                  tips={['Post a link in a chat', 'Invite your followers to vote']}
                  icon={FiTrendingUp}
                />
              ) : (
                <div className="home-card-grid">
                  {newThisWeek.map((matchup) => {
                    const matchupId = matchup.id ?? matchup.matchup_id ?? matchup.matchupId;
                    const matchupTitle = matchup.title || `Matchup #${matchupId}`;
                    const matchupOwnerId =
                      matchup.author_id ?? matchup.user_id ?? matchup.owner_id ?? userId;

                    return (
                      <motion.article
                        key={`new-${matchupId}`}
                        className="home-card home-card--soft"
                        whileHover={{ y: -4 }}
                      >
                        <div>
                          <h3>{matchupTitle}</h3>
                          <p>New debate | Just added</p>
                        </div>
                        {matchupOwnerId ? (
                          <Button
                            onClick={() =>
                              navigate(`/users/${matchupOwnerId}/matchup/${matchupId}`)
                            }
                            className="home-card-link"
                          >
                            Join the debate
                          </Button>
                        ) : (
                          <span className="home-card-link home-card-link--muted">Owner missing</span>
                        )}
                      </motion.article>
                    );
                  })}
                </div>
              )}
            </div>
          </div>

          <aside className="home-side">
            <div className="home-side-card">
              <h3>Start here</h3>
              <p>Make your first debate feel intentional.</p>
              <ul>
                <li>Pick a matchup template</li>
                <li>Invite two friends to vote</li>
                <li>Share the link in a group chat</li>
              </ul>
              <Button onClick={navigateToCreateMatchup} className="home-primary-button">
                Create matchup
              </Button>
            </div>

            <div className="home-side-card">
              <h3>Creators to follow</h3>
              <p>Community picks with strong engagement.</p>
              <div className="home-creator-list">
                {creatorSuggestions.map((creator, index) => (
                  <div key={creator.id ?? creator.key ?? index} className="home-creator-item">
                    <div>
                      <strong>{creator.name}</strong>
                      <span>{creator.tagline ?? 'Curated debates'}</span>
                    </div>
                    {creator.id ? (
                      <Button
                        onClick={() => navigate(`/users/${creator.username || creator.id}`)}
                        className="home-outline-button"
                      >
                        View
                      </Button>
                    ) : (
                      <span className="home-outline-button home-outline-button--muted">Soon</span>
                    )}
                  </div>
                ))}
              </div>
            </div>
          </aside>
        </Reveal>
      </main>
    </div>
  );
};

export default HomePage;
