import React, { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import ConfirmModal from '../components/ConfirmModal';
import {
  deleteCommunity,
  getCommunityBySlug,
  updateCommunity,
} from '../services/api';
import '../styles/CommunitySettings.css';

const MAX_NAME = 64;
const MAX_DESC = 500;
const MAX_TAGS = 10;

/*
 * CommunitySettings — owner-only edit form + delete affordance.
 *
 * Mods + members get a 403-style redirect back to the community
 * page on render (the link is owner-only in the UI but a direct
 * URL paste lands here). Backend still rejects non-owner writes;
 * this client-side gate is just for UX.
 */
const CommunitySettings = () => {
  const { slug } = useParams();
  const navigate = useNavigate();

  const [community, setCommunity] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [tagsInput, setTagsInput] = useState('');
  const [tags, setTags] = useState([]);

  const [saving, setSaving] = useState(false);
  const [savedAt, setSavedAt] = useState(null);
  const [submitError, setSubmitError] = useState(null);

  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    (async () => {
      try {
        const res = await getCommunityBySlug(slug);
        if (cancelled) return;
        const c = res?.data?.community ?? null;
        setCommunity(c);
        setName(c?.name || '');
        setDescription(c?.description || '');
        setTags(c?.tags || []);
      } catch (err) {
        if (cancelled) return;
        setError(err?.response?.status === 404 ? 'Community not found.' : 'Could not load this community.');
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [slug]);

  useEffect(() => {
    // Bounce non-owners back to the public community page. Backend
    // still gates the write so this is just UX.
    if (!loading && community && community.viewer_role !== 'owner') {
      navigate(`/c/${slug}`, { replace: true });
    }
  }, [loading, community, navigate, slug]);

  const addTagsFromInput = () => {
    const parts = tagsInput
      .split(',')
      .map((t) => t.trim().toLowerCase())
      .filter(Boolean);
    if (parts.length === 0) return;
    setTags((prev) => {
      const next = [...prev];
      for (const part of parts) {
        if (next.length >= MAX_TAGS) break;
        if (!next.includes(part) && /^[a-z0-9-]+$/.test(part)) {
          next.push(part);
        }
      }
      return next;
    });
    setTagsInput('');
  };

  const removeTag = (tag) => setTags((prev) => prev.filter((t) => t !== tag));

  const handleSave = async (e) => {
    e.preventDefault();
    if (!community || saving) return;
    setSubmitError(null);
    setSaving(true);
    try {
      const res = await updateCommunity(community.id, {
        name: name.trim(),
        description: description.trim(),
        tags,
      });
      const updated = res?.data?.community ?? null;
      if (updated) {
        setCommunity(updated);
        setSavedAt(new Date());
      }
    } catch (err) {
      const message =
        err?.response?.data?.message ||
        err?.message ||
        'Could not save changes. Please try again.';
      setSubmitError(message);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!community) return;
    setDeleting(true);
    try {
      await deleteCommunity(community.id);
      navigate('/home', { replace: true });
    } catch (err) {
      console.warn('Delete failed', err);
      alert('Could not delete community.');
      setDeleting(false);
    }
  };

  if (loading) {
    return (
      <div className="community-settings-page community-settings-page--loading">
        <div className="community-settings__spinner" aria-label="Loading settings" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="community-settings-page community-settings-page--error">
        <h1>{error}</h1>
        <Link to="/home">← Back to home</Link>
      </div>
    );
  }

  if (!community) return null;

  return (
    <div className="community-settings-page">
      <header className="community-settings__header">
        <p className="community-settings__overline">
          <Link to={`/c/${community.slug}`}>/c/{community.slug}</Link>
        </p>
        <h1 className="community-settings__title">Community settings</h1>
        <p className="community-settings__subtitle">
          Only the owner can edit these fields. Slug and privacy are fixed in v1.
        </p>
      </header>

      <form className="community-settings__form" onSubmit={handleSave}>
        <label className="community-settings__field">
          <span className="community-settings__label">Name</span>
          <input
            type="text"
            className="community-settings__input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={MAX_NAME}
            required
          />
          <span className="community-settings__help">{name.length}/{MAX_NAME}</span>
        </label>

        <label className="community-settings__field">
          <span className="community-settings__label">Description</span>
          <textarea
            className="community-settings__input community-settings__textarea"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            maxLength={MAX_DESC}
            rows={3}
          />
          <span className="community-settings__help">{description.length}/{MAX_DESC}</span>
        </label>

        <label className="community-settings__field">
          <span className="community-settings__label">Tags</span>
          <div className="community-settings__chips">
            {tags.map((tag) => (
              <span key={tag} className="community-settings__chip">
                {tag}
                <button
                  type="button"
                  className="community-settings__chip-remove"
                  onClick={() => removeTag(tag)}
                  aria-label={`Remove ${tag}`}
                >
                  ×
                </button>
              </span>
            ))}
            <input
              type="text"
              className="community-settings__chip-input"
              placeholder={tags.length ? '' : 'anime, music, sports'}
              value={tagsInput}
              onChange={(e) => setTagsInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === ',' || e.key === 'Enter') {
                  e.preventDefault();
                  addTagsFromInput();
                } else if (e.key === 'Backspace' && tagsInput === '' && tags.length > 0) {
                  setTags((prev) => prev.slice(0, -1));
                }
              }}
              onBlur={addTagsFromInput}
              disabled={tags.length >= MAX_TAGS}
            />
          </div>
          <span className="community-settings__help">{tags.length}/{MAX_TAGS} tags</span>
        </label>

        {submitError && (
          <div className="community-settings__error" role="alert">{submitError}</div>
        )}
        {savedAt && !submitError && (
          <div className="community-settings__success" role="status">Saved.</div>
        )}

        <div className="community-settings__actions">
          <Link to={`/c/${community.slug}`} className="community-settings__cancel">
            Back
          </Link>
          <button type="submit" className="community-settings__submit" disabled={saving}>
            {saving ? 'Saving…' : 'Save changes'}
          </button>
        </div>
      </form>

      <section className="community-settings__danger">
        <h2>Danger zone</h2>
        <p>
          Deleting the community soft-removes it for 30 days. Existing
          matchups and brackets that were posted in this community
          become standalone (their community link is severed).
        </p>
        <button
          type="button"
          className="community-settings__submit community-settings__submit--danger"
          onClick={() => setDeleteOpen(true)}
        >
          Delete community
        </button>
      </section>

      {deleteOpen && (
        <ConfirmModal
          title="Delete this community?"
          message="The community is hidden immediately and hard-deleted in 30 days. Matchups + brackets stay but lose their community link."
          confirmLabel={deleting ? 'Deleting…' : 'Delete community'}
          cancelLabel="Cancel"
          danger
          onConfirm={handleDelete}
          onCancel={() => setDeleteOpen(false)}
        />
      )}
    </div>
  );
};

export default CommunitySettings;
