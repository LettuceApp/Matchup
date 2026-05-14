import React, { useCallback, useEffect, useRef, useState } from 'react';
import { listMutuals, listMentionableMembers } from '../services/api';
import { extractActiveMentionQuery, replaceActiveMention } from '../utils/mentions';
import './MentionAutocomplete.css';

/*
 * MentionAutocomplete — drop-in companion to any controlled <textarea>
 * (or <input>) that wants @-mention autocomplete.
 *
 * Usage:
 *   <MentionAutocomplete
 *     value={text}
 *     onChange={(next, caret) => { setText(next); setCaret(caret); }}
 *     textareaRef={taRef}
 *     communityId={maybeCommunityId} // optional — switches the
 *                                    // suggestion source from mutuals
 *                                    // to community members
 *   />
 *
 * The parent owns the textarea + the value. This component:
 *   1. Watches the textarea for keystrokes + selection changes.
 *   2. When the caret sits inside an `@partial`, fetches matching
 *      users (debounced 120ms) and renders a positioned dropdown.
 *   3. ↑/↓ moves the highlight, Enter/Tab picks, Esc closes.
 *   4. On pick, replaces the active `@partial` with `@<username> `
 *      and calls onChange with the new value + caret position.
 *
 * Why an external component rather than baking this into each
 * composer: the mention behavior is identical in matchup comments,
 * bracket comments, and the future "user-as-item" picker. A single
 * component keeps the keyboard handling, debounce, and dropdown
 * rendering DRY.
 */
const DEBOUNCE_MS = 120;
const MAX_RESULTS = 8;

const MentionAutocomplete = ({ value, onChange, textareaRef, communityId }) => {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [results, setResults] = useState([]);
  const [activeIndex, setActiveIndex] = useState(0);
  // Caret state is read directly off the textarea on every event so
  // the parent doesn't have to thread it through. We only persist it
  // here to recompute the active-mention substring.
  const debounceRef = useRef(null);
  const fetchSeqRef = useRef(0);

  // Recompute the active mention whenever the caret moves or value
  // changes. Closes the dropdown when the user leaves the mention.
  const recompute = useCallback(() => {
    const ta = textareaRef.current;
    if (!ta) return;
    const partial = extractActiveMentionQuery(ta.value, ta.selectionStart);
    if (partial === null) {
      setOpen(false);
      setQuery('');
      return;
    }
    setQuery(partial);
    setOpen(true);
    setActiveIndex(0);
  }, [textareaRef]);

  // Fire `recompute` on relevant events.
  useEffect(() => {
    const ta = textareaRef.current;
    if (!ta) return undefined;
    const handler = () => recompute();
    ta.addEventListener('input', handler);
    ta.addEventListener('click', handler);
    ta.addEventListener('keyup', handler);
    return () => {
      ta.removeEventListener('input', handler);
      ta.removeEventListener('click', handler);
      ta.removeEventListener('keyup', handler);
    };
  }, [textareaRef, recompute]);

  // Debounced fetch — flips between mutuals and community members
  // based on whether the parent passed a `communityId`.
  useEffect(() => {
    if (!open) return undefined;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
      const mySeq = ++fetchSeqRef.current;
      try {
        const res = communityId
          ? await listMentionableMembers(communityId, { query, limit: MAX_RESULTS })
          : await listMutuals({ query, limit: MAX_RESULTS });
        if (mySeq !== fetchSeqRef.current) return; // stale
        setResults(res?.data?.users || []);
      } catch (err) {
        if (mySeq !== fetchSeqRef.current) return;
        console.warn('Mention autocomplete fetch failed', err);
        setResults([]);
      }
    }, DEBOUNCE_MS);
    return () => { if (debounceRef.current) clearTimeout(debounceRef.current); };
  }, [open, query, communityId]);

  const handlePick = useCallback((username) => {
    const ta = textareaRef.current;
    if (!ta) return;
    const { value: next, caret } = replaceActiveMention(ta.value, ta.selectionStart, username);
    onChange(next, caret);
    setOpen(false);
    setResults([]);
    // Restore focus + caret position after the parent's state update
    // commits — without this, the textarea loses focus on click-pick.
    requestAnimationFrame(() => {
      if (textareaRef.current) {
        textareaRef.current.focus();
        textareaRef.current.setSelectionRange(caret, caret);
      }
    });
  }, [textareaRef, onChange]);

  // Keyboard handling — attached to the textarea (capture phase) so
  // we can intercept Enter/Tab before the parent's submit handler
  // sees it.
  useEffect(() => {
    const ta = textareaRef.current;
    if (!ta) return undefined;
    const onKeyDown = (e) => {
      if (!open || results.length === 0) return;
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setActiveIndex((i) => (i + 1) % results.length);
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        setActiveIndex((i) => (i - 1 + results.length) % results.length);
      } else if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault();
        const pick = results[activeIndex];
        if (pick) handlePick(pick.username);
      } else if (e.key === 'Escape') {
        e.preventDefault();
        setOpen(false);
      }
    };
    ta.addEventListener('keydown', onKeyDown);
    return () => ta.removeEventListener('keydown', onKeyDown);
  }, [textareaRef, open, results, activeIndex, handlePick]);

  if (!open || results.length === 0) return null;
  return (
    <ul className="mention-autocomplete" role="listbox" aria-label="Mention suggestions">
      {results.map((u, idx) => (
        <li
          key={u.id}
          role="option"
          aria-selected={idx === activeIndex}
          className={`mention-autocomplete__item${idx === activeIndex ? ' mention-autocomplete__item--active' : ''}`}
          // mousedown (not click) so the textarea doesn't lose focus
          // first — onClick would race the textarea's blur handler.
          onMouseDown={(e) => { e.preventDefault(); handlePick(u.username); }}
          onMouseEnter={() => setActiveIndex(idx)}
        >
          {u.avatar_path ? (
            <img src={u.avatar_path} alt="" className="mention-autocomplete__avatar" />
          ) : (
            <span className="mention-autocomplete__avatar mention-autocomplete__avatar--initial">
              {(u.username || '?').charAt(0).toUpperCase()}
            </span>
          )}
          <span className="mention-autocomplete__username">@{u.username}</span>
        </li>
      ))}
    </ul>
  );
};

export default MentionAutocomplete;
