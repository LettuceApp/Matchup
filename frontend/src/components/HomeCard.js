import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { FiHeart, FiMessageCircle, FiShare2, FiArrowRight, FiCornerDownRight } from 'react-icons/fi';
import { relativeTime } from '../utils/time';
import { likeMatchup, unlikeMatchup, likeBracket, unlikeBracket } from '../services/api';
import { track } from '../utils/analytics';
import { useAnonUpgradePrompt } from '../contexts/AnonUpgradeContext';

// Calmer card-fallback gradients. Lower saturation than the
// previous palette so they read as quiet placeholders rather than
// decorative-noise — research feedback called the original 8-hue
// rotation out as competing with the actual content. All tints
// land in the same indigo / violet / slate cool range so the card
// grid feels cohesive even when no thumbnails have been uploaded.
const GRADIENTS = [
  'linear-gradient(135deg, rgba(99, 102, 241, 0.55), rgba(15, 23, 42, 0.95))',
  'linear-gradient(135deg, rgba(167, 139, 250, 0.5),  rgba(15, 23, 42, 0.95))',
  'linear-gradient(135deg, rgba(96, 165, 250, 0.55),  rgba(15, 23, 42, 0.95))',
  'linear-gradient(135deg, rgba(244, 114, 182, 0.45), rgba(15, 23, 42, 0.95))',
];

// Tag rules — order matters. The first rule whose keyword list hits
// the lowercased title contributes a tag (deriveTags returns up to 2
// matches). Specific subgenres come BEFORE broader categories so a
// title like "Best Pokémon Game" tags as Pokémon (not Gaming) and
// "K-Pop Song of the Year" tags as K-Pop (not Music).
//
// Tag strings must match the labels in HomeSidebar's CATEGORIES list
// — HomePage.js filters with an exact `tags.includes(categoryFilter)`.
const TAG_RULES = [
  // Specific subgenres (must precede the broader category they fall under).
  { keywords: ['pokemon', 'pokémon', 'pikachu', 'pokeball'], tag: 'Pokémon' },
  { keywords: ['k-pop', 'kpop', 'bts', 'blackpink', 'twice', 'newjeans'], tag: 'K-Pop' },
  { keywords: ['manga', 'manhwa', 'manhua', 'webtoon'], tag: 'Manga' },
  { keywords: ['anime', 'shonen', 'shoujo', 'op', 'ending'], tag: 'Anime' },
  { keywords: ['cartoon', 'looney', 'spongebob', 'simpsons', 'family guy', 'rick and morty', 'animated series'], tag: 'Cartoons' },

  // Broader categories.
  { keywords: ['music', 'song', 'songs', 'album', 'rapper', 'singer', 'band', 'artist', 'r&b', 'hip hop', 'rap'], tag: 'Music' },
  { keywords: ['movie', 'film', 'marvel', 'dc', 'cinema'], tag: 'Movies' },
  { keywords: ['tv', 'show', 'series', 'sitcom', 'episode', 'season'], tag: 'TV Shows' },
  { keywords: ['game', 'gaming', 'mario', 'xbox', 'playstation', 'nintendo', 'video game', 'rpg', 'fps'], tag: 'Gaming' },
  { keywords: ['sport', 'nba', 'nfl', 'soccer', 'football', 'basketball', 'baseball', 'goat', 'player'], tag: 'Sports' },
  { keywords: ['food', 'pizza', 'burger', 'taco', 'sushi', 'pasta', 'ramen', 'cuisine', 'restaurant', 'dish'], tag: 'Food' },
  { keywords: ['animal', 'dog', 'cat', 'pet', 'puppy', 'kitten', 'wolf', 'lion', 'bear'], tag: 'Animals' },
  { keywords: ['celebrity', 'celeb', 'actor', 'actress', 'kardashian', 'taylor swift', 'beyonce', 'rihanna'], tag: 'Celebrities' },
];

// Returns up to 2 tags for a title. When no rule fires we hand back
// ['Other'] — the sidebar pins an "Other" category as the catch-all,
// and the HomePage filter does an exact `tags.includes(categoryFilter)`,
// so the card display and the filter MUST share this vocabulary or
// the "Other" filter would never match anything.
export function deriveTags(title) {
  if (!title) return ['Other'];
  const lower = title.toLowerCase();
  const tags = [];
  for (const rule of TAG_RULES) {
    if (rule.keywords.some((kw) => lower.includes(kw))) {
      tags.push(rule.tag);
      if (tags.length === 2) break;
    }
  }
  return tags.length > 0 ? tags : ['Other'];
}

function titleGradient(title) {
  if (!title) return GRADIENTS[0];
  let hash = 0;
  for (let i = 0; i < title.length; i++) {
    hash = (hash * 31 + title.charCodeAt(i)) | 0;
  }
  return GRADIENTS[Math.abs(hash) % GRADIENTS.length];
}

// `relativeTime` now lives in utils/time.js so ActivityFeed + others
// can share it. Re-imported rather than duplicated.

function authorDisplay(item) {
  if (item.author?.username) return item.author.username;
  if (item.author_username) return item.author_username;
  const id = item.author_id ?? item.bracket_author_id;
  if (id) return id.slice(0, 8);
  return 'unknown';
}

function authorInitial(item) {
  const name = authorDisplay(item);
  return name.charAt(0).toUpperCase();
}

function withImageSize(url, size) {
  if (!url || !size) return url;
  return url.replace(/\.(jpe?g|png|gif|webp)$/i, `_${size}.jpg`);
}

const HomeCard = ({ item, type, initialLiked = false }) => {
  const navigate = useNavigate();
  const { promptUpgrade } = useAnonUpgradePrompt();
  const viewerId = typeof window !== 'undefined'
    ? window.localStorage.getItem('userId')
    : null;
  const title = item.title || (type === 'bracket' ? 'Untitled Bracket' : 'Untitled Matchup');
  const backendTags = Array.isArray(item.tags) && item.tags.length > 0 ? item.tags : null;
  // deriveTags() now guarantees at least one tag ('Other' as the
  // catch-all) so we can drop the previous in-component fallback —
  // and the chip we render is guaranteed to be a sidebar-known
  // category, so filtering on it works.
  const tags = backendTags ?? deriveTags(title);
  const gradient = titleGradient(title);
  const imageUrl = withImageSize(item.image_url ?? null, 'thumb');
  const author = authorDisplay(item);
  const initial = authorInitial(item);
  const timeAgo = relativeTime(item.created_at);
  const authorId = item.author_id ?? item.bracket_author_id;
  const initialLikesCount = item.likes_count ?? item.likes ?? 0;
  const commentsCount = Array.isArray(item.comments) ? item.comments.length : (item.comments ?? 0);
  const votesCount = item.votes ?? 0;

  // Interactive state for the action row. Initial liked-state arrives
  // as a prop (HomePage fetches the viewer's like list once on mount
  // and passes the relevant flag down). Optimistic + reversible —
  // failures roll the heart and count back. `burstKey` increments on
  // every like to remount the burst element so the CSS keyframe
  // re-runs (using `key={burstKey}` instead of toggling a class
  // sidesteps the "browser caches the animation state" gotcha).
  const [liked, setLiked] = useState(Boolean(initialLiked));
  const [likesCount, setLikesCount] = useState(initialLikesCount);
  const [likePending, setLikePending] = useState(false);
  const [burstKey, setBurstKey] = useState(0);
  const [shareCopied, setShareCopied] = useState(false);
  const roundLabel = item.round
    ? `Round ${item.round}`
    : (item.current_round ? `Round ${item.current_round}` : null);
  const contentText = item.content || '';
  // Bracket-child matchups need a parent-pointer subtitle. The titles
  // alone ("Round 1 - Match 4") read as gibberish in a flat feed without
  // the bracket name. `bracket_title` isn't always on the wire from the
  // home summary, so fall back to a generic "From a bracket" label.
  const isBracketChild = type === 'matchup' && Boolean(item.bracket_id);
  const parentBracketTitle = item.bracket_title ?? null;

  const handleClick = () => {
    if (type === 'bracket') {
      navigate(`/brackets/${item.id}`);
    } else {
      const uid = authorId ?? 'unknown';
      navigate(`/users/${uid}/matchup/${item.id}`);
    }
  };

  // Detail-page URL — same path the card click uses. Pulled out so
  // the share handler can build its absolute URL without recomputing.
  const detailPath = type === 'bracket'
    ? `/brackets/${item.id}`
    : `/users/${authorId ?? 'unknown'}/matchup/${item.id}`;

  // Stops the click from bubbling up to the card's onClick (which
  // would also navigate to detail). All three action buttons need
  // this; pulled out so each handler can `stop(e); ...` cleanly.
  const stop = (e) => {
    e.stopPropagation();
    e.preventDefault();
  };

  const handleLikeClick = async (e) => {
    stop(e);
    if (!viewerId) {
      // Anon viewers used to get a hard redirect to /login here. The
      // fade-background modal is friendlier — keeps the user on the
      // home feed and explains why they need to sign up to like.
      promptUpgrade('like');
      return;
    }
    if (likePending) return;
    setLikePending(true);
    const wasLiked = liked;
    // Optimistic flip. On a failure we roll both pieces of state back.
    setLiked(!wasLiked);
    setLikesCount((c) => Math.max(0, c + (wasLiked ? -1 : 1)));
    if (!wasLiked) setBurstKey((k) => k + 1);
    try {
      if (type === 'bracket') {
        await (wasLiked ? unlikeBracket(item.id) : likeBracket(item.id));
      } else {
        await (wasLiked ? unlikeMatchup(item.id) : likeMatchup(item.id));
      }
      track(
        type === 'bracket'
          ? (wasLiked ? 'bracket_unliked' : 'bracket_liked')
          : (wasLiked ? 'matchup_unliked' : 'matchup_liked'),
        { id: item.id, surface: 'home-feed' },
      );
    } catch (err) {
      console.error('like toggle failed', err);
      setLiked(wasLiked);
      setLikesCount((c) => Math.max(0, c + (wasLiked ? 1 : -1)));
    } finally {
      setLikePending(false);
    }
  };

  const handleCommentClick = (e) => {
    stop(e);
    // Comments live on the detail page — clicking here is just a
    // shortcut to "open the matchup so I can post a comment". A
    // dedicated `?focus=comments` query could scroll the textarea
    // into view; saving that for a later iteration.
    handleClick();
  };

  const handleShareClick = async (e) => {
    stop(e);
    const url = `${window.location.origin}${detailPath}`;
    track('content_shared', { id: item.id, type, surface: 'home-feed' });
    // Native share sheet on supporting devices (mobile + a few
    // desktop browsers); falls through to clipboard otherwise so
    // every platform gets some kind of share affordance.
    if (typeof navigator !== 'undefined' && typeof navigator.share === 'function') {
      try {
        await navigator.share({ title, url });
        return;
      } catch (err) {
        // User dismissed the sheet or the call rejected — fall
        // through to clipboard so the share still feels useful.
        if (err && err.name === 'AbortError') return;
      }
    }
    if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      try {
        await navigator.clipboard.writeText(url);
        setShareCopied(true);
        window.setTimeout(() => setShareCopied(false), 1800);
      } catch (err) {
        console.warn('clipboard write failed', err);
      }
    }
  };

  return (
    <article className="home-card" onClick={handleClick}>
      {/* Post header */}
      <div className="home-card__header">
        <span className="home-card__avatar">{initial}</span>
        <div className="home-card__header-text">
          <span className="home-card__username">{author}</span>
          {timeAgo && <span className="home-card__time">{timeAgo}</span>}
        </div>
        <span className="home-card__badge">
          {type === 'bracket' ? 'Bracket' : 'Matchup'}
        </span>
      </div>

      {/* Caption */}
      <div className="home-card__caption">
        {isBracketChild && (
          <div className="home-card__breadcrumb">
            <FiCornerDownRight aria-hidden="true" />
            <span>
              {parentBracketTitle ? `From: ${parentBracketTitle}` : 'From a bracket'}
            </span>
          </div>
        )}
        <h3 className="home-card__title">{title}</h3>
        {contentText && (
          <p className="home-card__content">{contentText}</p>
        )}
        {(votesCount > 0 || roundLabel) && (
          <div className="home-card__meta">
            {votesCount > 0 && (
              <span className="home-card__meta-item">
                {votesCount.toLocaleString()} {votesCount === 1 ? 'vote' : 'votes'}
              </span>
            )}
            {roundLabel && (
              <span className="home-card__meta-item">{roundLabel}</span>
            )}
          </div>
        )}
        <div className="home-card__tags">
          {tags.map((tag) => (
            <span key={tag} className="home-card__tag">#{tag}</span>
          ))}
        </div>
      </div>

      {/* Full-bleed media */}
      <div
        className="home-card__media"
        style={imageUrl ? {} : { background: gradient }}
      >
        {imageUrl && (
          <img
            src={imageUrl}
            alt={title}
            className="home-card__media-img"
            loading="lazy"
            decoding="async"
          />
        )}
        {/* Tap-to-vote chevron — appears on card hover (CSS) so the
            implicit "whole card is clickable" target gets an explicit
            visual cue. Research feedback flagged the lack of click
            affordance as making cards feel inert. */}
        <span className="home-card__cta" aria-hidden="true">
          <FiArrowRight />
        </span>
      </div>

      {/* Action bar — three real buttons. The card's outer onClick
          still navigates, but each button calls stop(e) so the click
          doesn't bubble up. Like toggles in place; Comment shortcuts
          to the detail page where the textarea lives; Share invokes
          the native share sheet (mobile) or copies to the clipboard
          (desktop fallback) and flips the label to "Copied!" briefly. */}
      <div className="home-card__actions">
        <div className="home-card__action-row">
          <button
            type="button"
            className={`home-card__action home-card__action--like${liked ? ' is-liked' : ''}`}
            onClick={handleLikeClick}
            disabled={likePending}
            aria-pressed={liked}
            aria-label={liked ? 'Unlike' : 'Like'}
          >
            <span className="home-card__action-icon">
              <FiHeart />
              {burstKey > 0 && (
                <span
                  key={burstKey}
                  className="home-card__like-burst"
                  aria-hidden="true"
                >
                  <FiHeart />
                </span>
              )}
            </span>
            {/* Action labels removed — icon + count is enough to read.
                The aria-label on each button still announces the
                action for screen readers. The shareCopied "Copied!"
                feedback is intentionally omitted; the share button
                briefly flashes the green hover state via CSS, which
                provides comparable confirmation without noise. */}
            {likesCount > 0 && <span className="home-card__action-count">{likesCount}</span>}
          </button>
          <button
            type="button"
            className="home-card__action home-card__action--comment"
            onClick={handleCommentClick}
            aria-label="Open to comment"
          >
            <FiMessageCircle />
            {commentsCount > 0 && <span className="home-card__action-count">{commentsCount}</span>}
          </button>
          <button
            type="button"
            className={`home-card__action home-card__action--share${shareCopied ? ' is-copied' : ''}`}
            onClick={handleShareClick}
            aria-label="Share"
          >
            <FiShare2 />
          </button>
        </div>
      </div>
    </article>
  );
};

export default HomeCard;
