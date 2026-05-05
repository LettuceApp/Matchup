import React, { useState, useEffect, useRef, useCallback } from 'react';
import { FiShare2, FiCopy, FiTwitter, FiFacebook, FiLinkedin, FiMail, FiMessageSquare } from 'react-icons/fi';
import '../styles/ShareButton.css';

/*
 * ShareButton — one-tap share for matchups and brackets.
 *
 * What "best executed" means here:
 *  - Mobile: use navigator.share() so the user gets their system share
 *    sheet (iMessage, Instagram DMs, whatever they have installed).
 *  - Desktop: dropdown with copy-link, Twitter/Facebook/LinkedIn
 *    intents, email, and SMS — the channels people actually use.
 *  - Pre-filled share text adapts to whether the viewer voted
 *    ("I'm team X — you?" reads way better than "check this out").
 *  - Every outgoing URL carries `?ref=<channel>` so attribution at
 *    landing time knows where the share came from.
 *  - Uses the backend short URL (`/m/<short_id>` or `/b/<short_id>`)
 *    so the preview actually renders when the link is pasted anywhere.
 */

// Build the full short URL from the item's short_id. Falls back to a
// long UUID-based path if short_id hasn't been backfilled yet — ugly
// but functional.
function buildShareUrl({ item, type, origin }) {
  const base = origin || (typeof window !== 'undefined' ? window.location.origin : '');
  const shortID = item?.short_id;
  if (shortID) {
    const prefix = type === 'bracket' ? '/b/' : '/m/';
    return `${base}${prefix}${shortID}`;
  }
  // Fallback — shareable but without the preview unless the caller has
  // configured legacy URL rewriting.
  if (type === 'bracket') {
    return `${base}/brackets/${item.id}`;
  }
  const uid =
    item.author?.username ||
    item.author_username ||
    item.author_id ||
    'u';
  return `${base}/users/${uid}/matchup/${item.id}`;
}

// Build the body text for a share. If the viewer has voted for an item,
// we bait a reply by naming their pick; otherwise it's a more generic
// prompt.
function buildShareText({ item, type, viewerVote }) {
  const title = item?.title || (type === 'bracket' ? 'this bracket' : 'this matchup');
  if (viewerVote?.itemLabel) {
    return `I'm team ${viewerVote.itemLabel} on "${title}" — you?`;
  }
  if (type === 'bracket') {
    return `Vote in this bracket: ${title}`;
  }
  return `Vote on this matchup: ${title}`;
}

// Attach a `?ref=<channel>` to a URL without clobbering existing query
// string. Used so the /share/landed endpoint on the backend can
// increment the right counter.
function withRef(url, ref) {
  if (!ref) return url;
  const sep = url.includes('?') ? '&' : '?';
  return `${url}${sep}ref=${encodeURIComponent(ref)}`;
}

const ShareButton = ({ item, type = 'matchup', viewerVote, className = '' }) => {
  const [open, setOpen] = useState(false);
  const [toast, setToast] = useState('');
  const [supportsNative, setSupportsNative] = useState(false);
  const rootRef = useRef(null);

  useEffect(() => {
    // Detect native share support once; treating it as reactive would
    // trigger unnecessary re-renders on platforms where the answer
    // is "no".
    setSupportsNative(typeof navigator !== 'undefined' && typeof navigator.share === 'function');
  }, []);

  // Dismiss the dropdown when the user clicks outside of it.
  useEffect(() => {
    if (!open) return undefined;
    const onClickOutside = (e) => {
      if (rootRef.current && !rootRef.current.contains(e.target)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', onClickOutside);
    return () => document.removeEventListener('mousedown', onClickOutside);
  }, [open]);

  const shareUrl = buildShareUrl({ item, type });
  const shareText = buildShareText({ item, type, viewerVote });

  const showToast = useCallback((msg) => {
    setToast(msg);
    setTimeout(() => setToast(''), 2000);
  }, []);

  const handleNativeShare = useCallback(async (e) => {
    e.stopPropagation();
    try {
      await navigator.share({
        title: item?.title || 'Matchup',
        text: shareText,
        url: withRef(shareUrl, 'native'),
      });
    } catch (err) {
      // User canceled the share sheet — not an error we surface.
      if (err.name !== 'AbortError') {
        // Fall back to opening the dropdown so they have other options.
        setOpen(true);
      }
    }
  }, [item, shareText, shareUrl]);

  const handleCopy = useCallback(async (e) => {
    e.stopPropagation();
    const url = withRef(shareUrl, 'copy');
    try {
      await navigator.clipboard.writeText(url);
      showToast('Link copied');
    } catch {
      // Clipboard API blocked — most likely because the doc isn't
      // focused or permissions weren't granted. Give them the raw URL
      // so at least it's recoverable.
      showToast('Copy failed — ' + url);
    }
    setOpen(false);
  }, [shareUrl, showToast]);

  const openIntent = useCallback((channel, href) => (e) => {
    e.stopPropagation();
    window.open(href, '_blank', 'noopener,noreferrer');
    setOpen(false);
  }, []);

  // Top-level click handler: on mobile (native share supported) go
  // straight to the native sheet; on desktop toggle the dropdown.
  const handleClick = useCallback((e) => {
    e.stopPropagation();
    if (supportsNative) {
      handleNativeShare(e);
    } else {
      setOpen((o) => !o);
    }
  }, [supportsNative, handleNativeShare]);

  const twitterHref = `https://twitter.com/intent/tweet?url=${encodeURIComponent(withRef(shareUrl, 'tw'))}&text=${encodeURIComponent(shareText)}`;
  const facebookHref = `https://www.facebook.com/sharer/sharer.php?u=${encodeURIComponent(withRef(shareUrl, 'fb'))}`;
  const linkedinHref = `https://www.linkedin.com/sharing/share-offsite/?url=${encodeURIComponent(withRef(shareUrl, 'li'))}`;
  const emailHref = `mailto:?subject=${encodeURIComponent(item?.title || 'Matchup')}&body=${encodeURIComponent(shareText + '\n\n' + withRef(shareUrl, 'email'))}`;
  const smsHref = `sms:?&body=${encodeURIComponent(shareText + ' ' + withRef(shareUrl, 'sms'))}`;

  return (
    <div className={`share-button-root ${className}`} ref={rootRef}>
      <button
        type="button"
        className="share-button"
        onClick={handleClick}
        aria-label="Share"
      >
        <FiShare2 />
        <span className="share-button__label">Share</span>
      </button>

      {!supportsNative && open && (
        <div className="share-menu" role="menu">
          <button type="button" className="share-menu__item" onClick={handleCopy} role="menuitem">
            <FiCopy /> <span>Copy link</span>
          </button>
          <button type="button" className="share-menu__item" onClick={openIntent('tw', twitterHref)} role="menuitem">
            <FiTwitter /> <span>X / Twitter</span>
          </button>
          <button type="button" className="share-menu__item" onClick={openIntent('fb', facebookHref)} role="menuitem">
            <FiFacebook /> <span>Facebook</span>
          </button>
          <button type="button" className="share-menu__item" onClick={openIntent('li', linkedinHref)} role="menuitem">
            <FiLinkedin /> <span>LinkedIn</span>
          </button>
          <button type="button" className="share-menu__item" onClick={openIntent('email', emailHref)} role="menuitem">
            <FiMail /> <span>Email</span>
          </button>
          <button type="button" className="share-menu__item" onClick={openIntent('sms', smsHref)} role="menuitem">
            <FiMessageSquare /> <span>SMS</span>
          </button>
        </div>
      )}

      {toast && <div className="share-toast">{toast}</div>}
    </div>
  );
};

export default ShareButton;
