import React, { useCallback, useEffect, useRef, useState } from 'react';
import {
  listMutuals,
  listMentionableMembers,
  search as universalSearch,
} from '../services/api';
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

// `source` selects the search backend:
//   - 'mutuals' (default): listMutuals — only mutuals; the original
//     comment-mention behaviour.
//   - 'all': universal search — broader, returns any matching user
//     once the query reaches 2 chars. Used by matchup item rows
//     where the goal is maximum shareability ("put anyone in your
//     matchup, not just mutuals").
// When `communityId` is set it OVERRIDES source — community
// composers always limit suggestions to the community's members
// regardless of what the parent passes.
//
// `enableMe` (default true) prepends an "@me — yourself" row when
// the query is empty or partial-prefix-matches "me". Backend
// resolves the @me handle to the viewer's user record on submit.
const MentionAutocomplete = ({ value, onChange, textareaRef, communityId, source = 'mutuals', enableMe = false }) => {
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

  // Debounced fetch. Source priority:
  //   1. communityId set → community members (existing behaviour;
  //      community composers always scope to the community)
  //   2. source === 'all' → universal user search (requires q >= 2)
  //   3. default → listMutuals (original behaviour)
  // Results are augmented with an @me synthetic row when enableMe is
  // on AND the user is typing nothing-or-"me-prefix" — appears before
  // any fetched matches so the creator's self-mention is the first
  // visible option.
  useEffect(() => {
    if (!open) return undefined;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
      const mySeq = ++fetchSeqRef.current;

      // Synthetic @me row. The id "__me__" is a sentinel the picker
      // recognises on select — the actual username sent on the wire
      // is "me", which the backend resolves to the viewer's user
      // record via resolveUserHandle's "me" branch.
      const meRow = enableMe && (!query || 'me'.startsWith(query.toLowerCase()))
        ? { id: '__me__', username: 'me', avatar_path: '', _isMe: true }
        : null;

      try {
        let fetched = [];
        if (communityId) {
          const res = await listMentionableMembers(communityId, { query, limit: MAX_RESULTS });
          fetched = res?.data?.users || [];
        } else if (source === 'all') {
          // Universal search requires q >= 2. Below that we just show
          // the @me row (if enabled) — no broad list yet.
          if ((query || '').length >= 2) {
            const res = await universalSearch({ query, only: 'user', limit: MAX_RESULTS });
            fetched = res?.data?.users || [];
          }
        } else {
          const res = await listMutuals({ query, limit: MAX_RESULTS });
          fetched = res?.data?.users || [];
        }
        if (mySeq !== fetchSeqRef.current) return; // stale
        const combined = meRow ? [meRow, ...fetched] : fetched;
        setResults(combined);
      } catch (err) {
        if (mySeq !== fetchSeqRef.current) return;
        console.warn('Mention autocomplete fetch failed', err);
        setResults(meRow ? [meRow] : []);
      }
    }, DEBOUNCE_MS);
    return () => { if (debounceRef.current) clearTimeout(debounceRef.current); };
  }, [open, query, communityId, source, enableMe]);

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
          {u._isMe && (
            <span className="mention-autocomplete__hint">yourself</span>
          )}
        </li>
      ))}
    </ul>
  );
};

export default MentionAutocomplete;
