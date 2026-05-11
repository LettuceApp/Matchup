import React from 'react';
import { useNavigate } from 'react-router-dom';
import '../styles/AnonUpgradeModal.css';

/*
 * AnonUpgradeModal — single dialog shown when an anonymous user hits
 * the vote cap (3rd vote) or attempts an action that requires an
 * account (e.g. clicking like, opening a comment box, voting on a
 * bracket).
 *
 * Two CTAs: Sign up (primary, sends to /register) and Log in (subtle,
 * sends to /login). Includes a reason prop so the same component can
 * render contextual copy:
 *
 *   reason="cap":      "You've used your 3 free votes — sign up to keep going."
 *   reason="bracket":  "Bracket voting is for members. Sign up free to join in."
 *   reason="like":     "Sign up to like this matchup and follow creators you back."
 *   reason="comment":  "Sign up to join the conversation."
 *
 * Defaults to "cap" — the most common trigger.
 */
const COPY = {
  cap: {
    title: 'Loved it?',
    body: 'Sign up free to keep voting + start your own matchups.',
  },
  bracket: {
    title: 'Brackets are for members',
    body: 'Sign up free to vote in tournaments and follow your favourites.',
  },
  like: {
    title: 'Sign up to like',
    body: 'Like matchups + follow creators you back. Always free.',
  },
  comment: {
    title: 'Join the conversation',
    body: 'Sign up free to post comments and reply to others.',
  },
  follow: {
    title: 'Follow this creator',
    body: 'Sign up free to follow creators and see their new matchups in your feed.',
  },
};

const AnonUpgradeModal = ({ reason = 'cap', onClose }) => {
  const navigate = useNavigate();
  const copy = COPY[reason] || COPY.cap;

  const goSignUp = () => {
    onClose?.();
    navigate('/register');
  };
  const goLogIn = () => {
    onClose?.();
    navigate('/login');
  };

  return (
    <div
      className="anon-upgrade-modal__scrim"
      role="dialog"
      aria-modal="true"
      aria-labelledby="anon-upgrade-title"
      onMouseDown={(e) => {
        // Click-out to dismiss. Only the scrim is clickable; clicks
        // inside the card propagate up but don't reach this handler
        // because the card calls e.stopPropagation in its own row.
        if (e.target === e.currentTarget) onClose?.();
      }}
    >
      <div className="anon-upgrade-modal__card" onMouseDown={(e) => e.stopPropagation()}>
        <h2 id="anon-upgrade-title" className="anon-upgrade-modal__title">
          {copy.title}
        </h2>
        <p className="anon-upgrade-modal__body">{copy.body}</p>
        <div className="anon-upgrade-modal__actions">
          <button
            type="button"
            className="anon-upgrade-modal__primary"
            onClick={goSignUp}
            autoFocus
          >
            Sign up free
          </button>
          <button
            type="button"
            className="anon-upgrade-modal__secondary"
            onClick={goLogIn}
          >
            Log in
          </button>
        </div>
        <button
          type="button"
          className="anon-upgrade-modal__dismiss"
          aria-label="Dismiss"
          onClick={() => {
            // "Maybe later" used to just close the modal and leave the
            // user on whichever gated page triggered it (bracket detail,
            // bracket-child matchup, etc.) — which was a dead-end since
            // the page's own data fetches had 401'd. Send them to the
            // homepage instead so they have somewhere to go that
            // actually renders for anon viewers.
            onClose?.();
            navigate('/');
          }}
        >
          Maybe later
        </button>
      </div>
    </div>
  );
};

export default AnonUpgradeModal;
