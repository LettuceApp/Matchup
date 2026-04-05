import React from 'react';
import { useNavigate } from 'react-router-dom';

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

function relativeTime(dateStr) {
  if (!dateStr) return '';
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 60) return `${mins || 1}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.floor(hrs / 24);
  return `${days}d ago`;
}

function authorDisplay(item) {
  // MatchupData (latest mode) has item.author.username
  if (item.author?.username) return item.author.username;
  // PopularMatchupData only has author_id UUID
  const id = item.author_id ?? item.bracket_author_id;
  if (id) return id.slice(0, 8);
  return 'unknown';
}

function authorInitial(item) {
  const name = authorDisplay(item);
  return name.charAt(0).toUpperCase();
}

const HomeCard = ({ item, type }) => {
  const navigate = useNavigate();
  const title = item.title || (type === 'bracket' ? 'Untitled Bracket' : 'Untitled Matchup');
  // Use real tags from API if available, fall back to keyword derivation
  const backendTags = Array.isArray(item.tags) && item.tags.length > 0 ? item.tags : null;
  const tags = backendTags ?? deriveTags(title);
  const gradient = titleGradient(title);
  const imageUrl = item.image_url ?? null;
  const author = authorDisplay(item);
  const initial = authorInitial(item);
  const timeAgo = relativeTime(item.created_at);
  const votes = item.votes ?? item.likes_count ?? 0;
  const authorId = item.author_id ?? item.bracket_author_id;

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
      <div
        className="home-card__thumb"
        style={imageUrl ? {} : { background: gradient }}
      >
        {imageUrl && (
          <img
            src={imageUrl}
            alt={title}
            className="home-card__thumb-img"
          />
        )}
        <span className="home-card__badge">
          {type === 'bracket' ? 'Bracket' : 'Matchup'}
        </span>
      </div>

      <div className="home-card__body">
        <h3 className="home-card__title">{title}</h3>

        <div className="home-card__author">
          <span className="home-card__avatar">{initial}</span>
          <span className="home-card__username">{author}</span>
          {timeAgo && <span className="home-card__time">&middot; {timeAgo}</span>}
        </div>

        <div className="home-card__stats">
          <span>{votes} participants</span>
        </div>

        {tags.length > 0 && (
          <div className="home-card__tags">
            {tags.map((tag) => (
              <span key={tag} className="home-card__tag">{tag}</span>
            ))}
          </div>
        )}
      </div>
    </article>
  );
};

export default HomeCard;
