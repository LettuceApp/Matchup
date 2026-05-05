import React, { useEffect, useRef, useState } from 'react';
import { FiMoreHorizontal, FiFlag, FiVolumeX, FiVolume2, FiShield, FiShieldOff } from 'react-icons/fi';
import { blockUser, unblockUser, muteUser, unmuteUser } from '../services/api';
import ReportModal from './ReportModal';
import '../styles/BlockMuteMenu.css';

/*
 * BlockMuteMenu — overflow menu next to the Follow button on a user
 * profile header. Bundles the three moderation actions (Report, Mute,
 * Block) + their opposites into one affordance so the header doesn't
 * sprout three separate buttons.
 *
 * Props:
 *   targetId        username or public UUID, whatever the caller has
 *   targetUserUuid  UUID specifically, needed by ReportModal which
 *                   requires a subject_id that's always a UUID. Falls
 *                   back to targetId if caller passed a UUID there too.
 *   initiallyBlocked / initiallyMuted  initial state from relationship
 *                   info; component mirrors updates optimistically.
 *   onStateChange(next)  called with { blocked, muted } after any
 *                   successful mutation. Lets the parent hide the
 *                   rest of the profile (matchups, likes) when the
 *                   viewer blocks the target.
 *
 * Design intent: one dropdown, positive-state + inverse-state labels
 * swap in place, confirmations only on Block (the destructive one —
 * it severs follows both ways server-side). Mute + Report are quiet.
 */
const BlockMuteMenu = ({
  targetId,
  targetUserUuid,
  initiallyBlocked = false,
  initiallyMuted = false,
  onStateChange,
}) => {
  const [open, setOpen] = useState(false);
  const [blocked, setBlocked] = useState(initiallyBlocked);
  const [muted, setMuted] = useState(initiallyMuted);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [reportOpen, setReportOpen] = useState(false);
  const containerRef = useRef(null);

  // Sync local state when parent resolves the relationship later —
  // BlockMuteMenu often mounts before relationship data arrives.
  useEffect(() => { setBlocked(initiallyBlocked); }, [initiallyBlocked]);
  useEffect(() => { setMuted(initiallyMuted); }, [initiallyMuted]);

  // Outside-click dismiss — the menu is small, easy to lose if the
  // user changes their mind and clicks away.
  useEffect(() => {
    if (!open) return undefined;
    const handler = (e) => {
      if (containerRef.current && !containerRef.current.contains(e.target)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  const commit = (next) => {
    setBlocked(Boolean(next.blocked));
    setMuted(Boolean(next.muted));
    onStateChange?.(next);
  };

  const handleBlockToggle = async () => {
    setError(null);
    if (busy) return;
    if (!blocked) {
      const ok = window.confirm(
        'Block this user? This removes any follow edges between you both and hides their content from you everywhere.'
      );
      if (!ok) return;
    }
    setBusy(true);
    try {
      if (blocked) {
        await unblockUser(targetId);
        commit({ blocked: false, muted });
      } else {
        await blockUser(targetId);
        // Server clears any mute when blocking — mirror that locally.
        commit({ blocked: true, muted: false });
      }
      setOpen(false);
    } catch (err) {
      console.error('Block toggle failed', err);
      setError('Could not update block. Try again in a moment.');
    } finally {
      setBusy(false);
    }
  };

  const handleMuteToggle = async () => {
    setError(null);
    if (busy) return;
    setBusy(true);
    try {
      if (muted) {
        await unmuteUser(targetId);
        commit({ blocked, muted: false });
      } else {
        await muteUser(targetId);
        commit({ blocked, muted: true });
      }
      setOpen(false);
    } catch (err) {
      console.error('Mute toggle failed', err);
      setError('Could not update mute. Try again in a moment.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="block-mute-menu" ref={containerRef}>
      <button
        type="button"
        className="block-mute-menu__trigger"
        aria-label="More actions"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        <FiMoreHorizontal />
      </button>

      {open && (
        <div className="block-mute-menu__panel" role="menu">
          <button
            type="button"
            className="block-mute-menu__item"
            role="menuitem"
            onClick={() => { setOpen(false); setReportOpen(true); }}
          >
            <FiFlag /> Report…
          </button>
          <button
            type="button"
            className="block-mute-menu__item"
            role="menuitem"
            disabled={busy}
            onClick={handleMuteToggle}
          >
            {muted ? <><FiVolume2 /> Unmute</> : <><FiVolumeX /> Mute</>}
          </button>
          <button
            type="button"
            className={`block-mute-menu__item ${blocked ? '' : 'block-mute-menu__item--danger'}`}
            role="menuitem"
            disabled={busy}
            onClick={handleBlockToggle}
          >
            {blocked ? <><FiShieldOff /> Unblock</> : <><FiShield /> Block</>}
          </button>
          {error && <p className="block-mute-menu__error">{error}</p>}
        </div>
      )}

      {reportOpen && (
        <ReportModal
          subjectType="user"
          subjectId={targetUserUuid || targetId}
          onClose={() => setReportOpen(false)}
        />
      )}
    </div>
  );
};

export default BlockMuteMenu;
