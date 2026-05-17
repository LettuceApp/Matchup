import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { checkSlugAvailable, createCommunity } from '../services/api';
import '../styles/CreateCommunity.css';

const MIN_NAME_LEN = 2;
const MAX_NAME_LEN = 64;
const MAX_DESC_LEN = 500;
const MAX_TAGS = 10;

// Slug check is debounced so the field doesn't fire an RPC per
// keystroke. 350ms feels responsive without being chatty.
const SLUG_CHECK_DEBOUNCE_MS = 350;

// User-facing copy says "URL name" everywhere "slug" used to. The
// internal identifier in code + DB stays `slug` because the field
// name shows up in protos, indexes, and routing — renaming there
// would be a much bigger surgery for the same UX outcome.
const reasonToCopy = {
  invalid: 'Use 3–32 lowercase letters, numbers, or hyphens. No leading or trailing hyphen.',
  reserved: 'That URL name is reserved. Try another one.',
  taken: 'That URL name is already taken.',
};

/*
 * Slugify helper — best-effort transformation of a free-form name
 * into something that passes the server's slug regex. We never
 * silently rename what the user typed in the SLUG field, but we DO
 * use this to suggest a slug as the user types their name. The user
 * can edit/override the suggestion at any time.
 */
const slugifyName = (name) =>
  name
    .toLowerCase()
    .normalize('NFKD')
    .replace(/[^\w\s-]/g, '')
    .trim()
    .replace(/[\s_]+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 32);

const CreateCommunity = () => {
  const navigate = useNavigate();
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  // slugTouched flips true the first time the user edits the slug
  // field directly. After that we stop overwriting their value when
  // they keep typing in the name field — they're driving the slug.
  const [slugTouched, setSlugTouched] = useState(false);
  const [description, setDescription] = useState('');
  const [tagsInput, setTagsInput] = useState('');
  const [tags, setTags] = useState([]);

  // Slug availability state: 'idle' | 'checking' | 'available' | 'unavailable'
  const [slugState, setSlugState] = useState('idle');
  const [slugReason, setSlugReason] = useState('');
  const [slugSuggestions, setSlugSuggestions] = useState([]);

  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState(null);

  const debounceTimer = useRef(null);

  // Mirror the user's name input into the slug field until they take
  // it over. This keeps the slug aligned with the name during the
  // initial type-out without yanking edits from under them later.
  useEffect(() => {
    if (slugTouched) return;
    setSlug(slugifyName(name));
  }, [name, slugTouched]);

  // Debounced slug-availability check. Re-fires whenever slug changes
  // AND the slug is non-empty. The check itself is cheap — single
  // DB EXISTS query — so we don't bother with caching.
  useEffect(() => {
    if (debounceTimer.current) {
      clearTimeout(debounceTimer.current);
    }
    if (!slug) {
      setSlugState('idle');
      setSlugReason('');
      setSlugSuggestions([]);
      return undefined;
    }
    setSlugState('checking');
    debounceTimer.current = setTimeout(async () => {
      try {
        const res = await checkSlugAvailable(slug);
        const payload = res?.data ?? {};
        if (payload.available) {
          setSlugState('available');
          setSlugReason('');
          setSlugSuggestions([]);
        } else {
          setSlugState('unavailable');
          setSlugReason(payload.reason || '');
          setSlugSuggestions(payload.suggestions || []);
        }
      } catch (err) {
        // Network failure — leave UI in checking state so the user
        // doesn't get a false "available" signal. Submit will surface
        // the real error.
        console.warn('Slug check failed', err);
      }
    }, SLUG_CHECK_DEBOUNCE_MS);
    return () => clearTimeout(debounceTimer.current);
  }, [slug]);

  // Whether the last attempted tag-add was rejected for matching
  // the community name. Drives the inline hint below the chip row.
  const [tagNameEcho, setTagNameEcho] = useState(false);

  const addTagsFromInput = useCallback(() => {
    const parts = tagsInput
      .split(',')
      .map((t) => t.trim().toLowerCase())
      .filter(Boolean);
    if (parts.length === 0) return;
    const normalizedName = name.trim().toLowerCase();
    let rejectedNameEcho = false;
    setTags((prev) => {
      const next = [...prev];
      for (const part of parts) {
        if (next.length >= MAX_TAGS) break;
        // Reject tags that just echo the community name — they're
        // always redundant with the title. Server re-validates +
        // strips on save, but catching it here means the user sees
        // the inline hint immediately instead of getting a confusing
        // "your tag disappeared after save".
        if (normalizedName && part === normalizedName) {
          rejectedNameEcho = true;
          continue;
        }
        // Skip duplicates + values with unsafe characters; server
        // re-validates so this is just for UX.
        if (!next.includes(part) && /^[a-z0-9-]+$/.test(part)) {
          next.push(part);
        }
      }
      return next;
    });
    setTagsInput('');
    setTagNameEcho(rejectedNameEcho);
  }, [tagsInput, name]);

  const handleTagsKeyDown = (e) => {
    // Comma + Enter both confirm a tag. Backspace on an empty input
    // removes the last chip — standard chip-input pattern.
    if (e.key === ',' || e.key === 'Enter') {
      e.preventDefault();
      addTagsFromInput();
    } else if (e.key === 'Backspace' && tagsInput === '' && tags.length > 0) {
      setTags((prev) => prev.slice(0, -1));
    }
  };

  const removeTag = (tag) => setTags((prev) => prev.filter((t) => t !== tag));

  const formValid = useMemo(() => {
    if (name.trim().length < MIN_NAME_LEN || name.trim().length > MAX_NAME_LEN) return false;
    if (slug.length < 3 || slug.length > 32) return false;
    if (slugState !== 'available') return false;
    if (description.length > MAX_DESC_LEN) return false;
    return true;
  }, [name, slug, slugState, description]);

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!formValid || submitting) return;
    setSubmitting(true);
    setSubmitError(null);
    try {
      const res = await createCommunity({
        slug,
        name: name.trim(),
        description: description.trim(),
        tags,
        privacy: 'public',
      });
      const created = res?.data?.community ?? res?.data ?? {};
      const createdSlug = created.slug || slug;
      navigate(`/c/${createdSlug}`, { replace: true });
    } catch (err) {
      const message =
        err?.response?.data?.message ||
        err?.message ||
        'We could not create the community. Please try again.';
      setSubmitError(message);
    } finally {
      setSubmitting(false);
    }
  };

  // Slug-input className reflects the live availability state.
  const slugFieldClass = [
    'community-create-input',
    'community-create-input--slug',
    slugState === 'available' ? 'is-available' : '',
    slugState === 'unavailable' ? 'is-unavailable' : '',
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <div className="community-create-page">
      <div className="community-create-card">
        <header className="community-create-header">
          <h1>Create a community</h1>
          <p>
            Spin up a space for tournaments, debates, and matchups around any topic.
            Anyone can join a public community and vote on its matchups.
          </p>
        </header>

        <form className="community-create-form" onSubmit={handleSubmit}>
          <label className="community-create-field">
            <span className="community-create-label">Name</span>
            <input
              type="text"
              className="community-create-input"
              placeholder="e.g. Anime Fans"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={MAX_NAME_LEN}
              required
            />
            <span className="community-create-help">
              {name.length}/{MAX_NAME_LEN}
            </span>
          </label>

          <label className="community-create-field">
            <span className="community-create-label">URL name</span>
            <div className="community-create-slug-row">
              <span className="community-create-slug-prefix">/c/</span>
              <input
                type="text"
                className={slugFieldClass}
                placeholder="anime-fans"
                value={slug}
                onChange={(e) => {
                  setSlugTouched(true);
                  setSlug(e.target.value.toLowerCase());
                }}
                minLength={3}
                maxLength={32}
                required
              />
            </div>
            <span className="community-create-help">
              {slug && slugState === 'checking' && 'Checking…'}
              {slug && slugState === 'available' && (
                <span className="community-create-help--ok">Available</span>
              )}
              {slug && slugState === 'unavailable' && (
                <span className="community-create-help--err">
                  {reasonToCopy[slugReason] || 'Not available'}
                </span>
              )}
              {!slug && 'Lowercase letters, numbers, hyphens. 3–32 chars.'}
            </span>
            {slugSuggestions.length > 0 && (
              <div className="community-create-suggestions">
                <span>Try:</span>
                {slugSuggestions.map((s) => (
                  <button
                    key={s}
                    type="button"
                    className="community-create-suggestion"
                    onClick={() => {
                      setSlugTouched(true);
                      setSlug(s);
                    }}
                  >
                    {s}
                  </button>
                ))}
              </div>
            )}
          </label>

          <label className="community-create-field">
            <span className="community-create-label">Description</span>
            <textarea
              className="community-create-input community-create-textarea"
              placeholder="What is this community about? Who is it for? (optional, but if filled, at least 20 chars)"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              maxLength={MAX_DESC_LEN}
              rows={3}
            />
            <span className="community-create-help">
              {description.length}/{MAX_DESC_LEN}
            </span>
          </label>

          <label className="community-create-field">
            <span className="community-create-label">Tags</span>
            <div className="community-create-tags-chips">
              {tags.map((tag) => (
                <span key={tag} className="community-create-tag">
                  {tag}
                  <button
                    type="button"
                    className="community-create-tag-remove"
                    onClick={() => removeTag(tag)}
                    aria-label={`Remove ${tag}`}
                  >
                    ×
                  </button>
                </span>
              ))}
              <input
                type="text"
                className="community-create-input community-create-tags-input"
                placeholder={tags.length ? '' : 'anime, music, sports (separate with commas)'}
                value={tagsInput}
                onChange={(e) => setTagsInput(e.target.value)}
                onKeyDown={handleTagsKeyDown}
                onBlur={addTagsFromInput}
                disabled={tags.length >= MAX_TAGS}
              />
            </div>
            <span className="community-create-help">
              {tagNameEcho ? (
                <span className="community-create-help--err">
                  That tag matches the community name — try a more specific tag.
                </span>
              ) : (
                <>{tags.length}/{MAX_TAGS} tags. Press Enter or comma to add.</>
              )}
            </span>
          </label>

          {submitError && (
            <div className="community-create-error" role="alert">
              {submitError}
            </div>
          )}

          <div className="community-create-actions">
            <Link to="/home" className="community-create-cancel">
              Cancel
            </Link>
            <button
              type="submit"
              className="community-create-submit"
              disabled={!formValid || submitting}
            >
              {submitting ? 'Creating…' : 'Create community'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};

export default CreateCommunity;
