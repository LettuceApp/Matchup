import React from 'react';
import { useNavigate } from 'react-router-dom';
import { FiHeart, FiMessageCircle, FiShare2 } from 'react-icons/fi';
import { relativeTime } from '../utils/time';

const GRADIENTS = [
  'linear-gradient(135deg, #667eea, #764ba2)',
  'linear-gradient(135deg, #f093fb, #f5576c)',
  'linear-gradient(135deg, #4facfe, #00f2fe)',
  'linear-gradient(135deg, #43e97b, #38f9d7)',
  'linear-gradient(135deg, #fa709a, #fee140)',
  'linear-gradient(135deg, #a18cd1, #fbc2eb)',
  'linear-gradient(135deg, #fccb90, #d57eeb)',
  'linear-gradient(135deg, #a1c4fd, #c2e9fb)',
];

const TAG_RULES = [
  { keywords: ['music', 'song', 'songs', 'album', 'rapper', 'singer', 'band', 'artist', 'r&b', 'hip hop', 'rap'], tag: 'Music' },
  { keywords: ['anime', 'manga', 'op', 'ending', 'k-pop', 'kpop'], tag: 'Anime' },
  { keywords: ['game', 'gaming', 'pokemon', 'mario', 'xbox', 'playstation', 'nintendo', 'video game'], tag: 'Gaming' },
  { keywords: ['movie', 'film', 'tv', 'show', 'series', 'marvel', 'dc', 'character', 'villain', 'hero'], tag: 'Movies/TV' },
  { keywords: ['sport', 'nba', 'nfl', 'soccer', 'football', 'basketball', 'baseball', 'goat', 'player'], tag: 'Sports' },
];

export function deriveTags(title) {
  if (!title) return [];
  const lower = title.toLowerCase();
  const tags = [];
  for (const rule of TAG_RULES) {
    if (rule.keywords.some((kw) => lower.includes(kw))) {
      tags.push(rule.tag);
      if (tags.length === 2) break;
    }
  }
  return tags;
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

const HomeCard = ({ item, type }) => {
  const navigate = useNavigate();
  const title = item.title || (type === 'bracket' ? 'Untitled Bracket' : 'Untitled Matchup');
  const backendTags = Array.isArray(item.tags) && item.tags.length > 0 ? item.tags : null;
  const tags = backendTags ?? deriveTags(title);
  const gradient = titleGradient(title);
  const imageUrl = withImageSize(item.image_url ?? null, 'thumb');
  const author = authorDisplay(item);
  const initial = authorInitial(item);
  const timeAgo = relativeTime(item.created_at);
  const authorId = item.author_id ?? item.bracket_author_id;
  const likesCount = item.likes_count ?? item.likes ?? 0;
  const commentsCount = Array.isArray(item.comments) ? item.comments.length : (item.comments ?? 0);
  const contentText = item.content || '';

  const handleClick = () => {
    if (type === 'bracket') {
      navigate(`/brackets/${item.id}`);
    } else {
      const uid = authorId ?? 'unknown';
      navigate(`/users/${uid}/matchup/${item.id}`);
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
        <h3 className="home-card__title">{title}</h3>
        {contentText && (
          <p className="home-card__content">{contentText}</p>
        )}
        {tags.length > 0 && (
          <div className="home-card__tags">
            {tags.map((tag) => (
              <span key={tag} className="home-card__tag">#{tag}</span>
            ))}
          </div>
        )}
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
      </div>

      {/* Action bar */}
      <div className="home-card__actions">
        <div className="home-card__action-row">
          <span className="home-card__action">
            <FiHeart /> {likesCount > 0 && likesCount} Like
          </span>
          <span className="home-card__action">
            <FiMessageCircle /> {commentsCount > 0 && commentsCount} Comment
          </span>
          <span className="home-card__action">
            <FiShare2 /> Share
          </span>
        </div>
      </div>
    </article>
  );
};

export default HomeCard;
