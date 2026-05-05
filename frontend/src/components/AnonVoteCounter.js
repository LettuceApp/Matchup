import React from 'react';
import { Link } from 'react-router-dom';
import '../styles/AnonVoteCounter.css';

/*
 * AnonVoteCounter — small chip rendered next to the vote action on
 * matchup surfaces when the viewer is anonymous. Drives the conversion
 * funnel: every successful vote ticks the counter down, and the third
 * vote flips the chip into a sign-up CTA.
 *
 * Props:
 *   used:      current count from useAnonVoteStatus
 *   max:       cap from useAnonVoteStatus
 *   atCap:     boolean shorthand for used >= max
 *   onPromptSignup:  optional callback when the user clicks the
 *                    at-cap chip (the parent typically opens the
 *                    AnonUpgradeModal). Falls back to the /register
 *                    Link when omitted.
 *
 * Visual: amber → red gradient as remaining votes decrease, signalling
 * urgency without being an error state.
 */
const AnonVoteCounter = ({ used, max, atCap, onPromptSignup }) => {
  if (atCap) {
    const inner = (
      <>
        <span aria-hidden="true">⚡</span> Vote limit reached — Sign up
      </>
    );
    if (onPromptSignup) {
      return (
        <button
          type="button"
          className="anon-vote-counter anon-vote-counter--cap"
          onClick={onPromptSignup}
        >
          {inner}
        </button>
      );
    }
    return (
      <Link to="/register" className="anon-vote-counter anon-vote-counter--cap">
        {inner}
      </Link>
    );
  }

  const remaining = Math.max(max - used, 0);
  const variant = remaining === 1 ? 'warn' : 'ok';
  return (
    <span
      className={`anon-vote-counter anon-vote-counter--${variant}`}
      aria-live="polite"
    >
      {remaining} of {max} free votes
    </span>
  );
};

export default AnonVoteCounter;
