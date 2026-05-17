import React, { useEffect } from 'react';
import { Link } from 'react-router-dom';
import { FiX, FiUser } from 'react-icons/fi';
import '../styles/AudienceModal.css';

/*
 * AudienceModal — owner-only panel that lists who voted or who liked
 * a matchup / bracket. Renders inline on the detail page (not via a
 * portal) the same way ConfirmModal does, so the surrounding scroll
 * lock behaves predictably. Closed via the X button, the backdrop
 * click, or Escape.
 *
 * Props:
 *   open        — boolean; renders nothing when false (so parents can
 *                 keep the trigger button mounted without paying for
 *                 the modal DOM).
 *   onClose     — () => void; called for backdrop / X / Esc.
 *   title       — header text, e.g. "Voters" or "Likers".
 *   loading     — show a small "Loading…" line in place of the list.
 *   error       — error string; shown above the list when set.
 *   users       — array of { id, username, avatar_path, pickedItemLabel? }.
 *   anonCount   — optional integer; renders a single grouped row at
 *                 the bottom of the list when > 0 (voters panel only).
 *   emptyLabel  — copy for the empty-list state.
 *
 * Avatars use the resolved full S3 URL the server already populates
 * (UserSummaryResponse.avatar_path is lifted via ProcessAvatarPath
 * upstream), so this component renders <img src={…}> directly.
 */
const AudienceModal = ({
  open,
  onClose,
  title,
  loading = false,
  error = null,
  users = [],
  anonCount = 0,
  emptyLabel = 'No one yet.',
}) => {
  // Esc-to-close. Bound only while open so closed-modal mounts don't
  // leak keyboard listeners.
  useEffect(() => {
    if (!open) return undefined;
    const onKey = (e) => {
      if (e.key === 'Escape') onClose?.();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      className="audience-modal__overlay"
      onClick={onClose}
      role="presentation"
    >
      <div
        className="audience-modal"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        // Stop propagation so clicks inside the panel don't dismiss
        // via the overlay's onClick.
        onClick={(e) => e.stopPropagation()}
      >
        <header className="audience-modal__header">
          <h2 className="audience-modal__title">{title}</h2>
          <button
            type="button"
            className="audience-modal__close"
            onClick={onClose}
            aria-label="Close"
          >
            <FiX />
          </button>
        </header>

        <div className="audience-modal__body">
          {loading && <p className="audience-modal__state">Loading…</p>}
          {error && !loading && (
            <p className="audience-modal__state audience-modal__state--error">{error}</p>
          )}
          {!loading && !error && users.length === 0 && anonCount === 0 && (
            <p className="audience-modal__state">{emptyLabel}</p>
          )}

          {users.length > 0 && (
            <ul className="audience-modal__list">
              {users.map((u) => (
                <li key={u.id} className="audience-modal__row">
                  <Link
                    to={`/users/${u.username}`}
                    className="audience-modal__user"
                    onClick={onClose}
                  >
                    {u.avatar_path ? (
                      <img
                        src={u.avatar_path}
                        alt=""
                        className="audience-modal__avatar"
                      />
                    ) : (
                      <span className="audience-modal__avatar audience-modal__avatar--fallback">
                        {(u.username || '?').charAt(0).toUpperCase()}
                      </span>
                    )}
                    <span className="audience-modal__username">@{u.username}</span>
                  </Link>
                  {u.pickedItemLabel && (
                    <span className="audience-modal__pick">
                      voted <strong>{u.pickedItemLabel}</strong>
                    </span>
                  )}
                </li>
              ))}
            </ul>
          )}

          {/* Single grouped row for anonymous voters. Greyed silhouette
              + count, no individual entries — see product decision
              comment in audience_handlers.go's GetMatchupVoters. */}
          {anonCount > 0 && (
            <div className="audience-modal__anon">
              <span className="audience-modal__avatar audience-modal__avatar--anon" aria-hidden="true">
                <FiUser />
              </span>
              <span>
                <strong>{anonCount}</strong>{' '}
                {anonCount === 1 ? 'anonymous voter' : 'anonymous voters'}
              </span>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default AudienceModal;
