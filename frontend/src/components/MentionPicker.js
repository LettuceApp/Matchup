import React, { useCallback, useEffect, useRef, useState } from 'react';
import { listMutuals, listMentionableMembers } from '../services/api';
import './MentionPicker.css';

/*
 * MentionPicker — standalone user picker for places that need a
 * single "pick one user" affordance (matchup item rows on the create
 * form, future contender slots on brackets, etc).
 *
 * Different from MentionAutocomplete: this isn't anchored to a
 * textarea. It renders its own search input, fires the same
 * listMutuals / listMentionableMembers RPC depending on whether the
 * parent passed a communityId, and emits onSelect(user) when the
 * user clicks a result.
 *
 * Once a user is picked, the parent flips `selected` to non-null and
 * the picker collapses into a chip showing avatar + @username + a
 * clear button. Clicking the chip's × calls onClear and restores the
 * input.
 *
 * Layout: position relative on the root so the absolute-positioned
 * dropdown anchors against it. Parent doesn't need any special
 * styling — drop it into a flex row alongside other controls.
 */
const DEBOUNCE_MS = 120;
const MAX_RESULTS = 6;

const MentionPicker = ({ selected, onSelect, onClear, communityId, placeholder }) => {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState([]);
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const debounceRef = useRef(null);
  const fetchSeqRef = useRef(0);
  const inputRef = useRef(null);
  const rootRef = useRef(null);

  // Close on outside click — same recipe as the avatar dropdown menus.
  useEffect(() => {
    if (!open) return undefined;
    const onPointerDown = (e) => {
      if (rootRef.current && !rootRef.current.contains(e.target)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', onPointerDown);
    return () => document.removeEventListener('mousedown', onPointerDown);
  }, [open]);

  // Debounced fetch — same shape as MentionAutocomplete, just with
  // the standalone input feeding the query.
  useEffect(() => {
    if (!open) return undefined;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
      const mySeq = ++fetchSeqRef.current;
      try {
        const res = communityId
          ? await listMentionableMembers(communityId, { query, limit: MAX_RESULTS })
          : await listMutuals({ query, limit: MAX_RESULTS });
        if (mySeq !== fetchSeqRef.current) return;
        setResults(res?.data?.users || []);
        setActiveIndex(0);
      } catch (err) {
        if (mySeq !== fetchSeqRef.current) return;
        console.warn('MentionPicker fetch failed', err);
        setResults([]);
      }
    }, DEBOUNCE_MS);
    return () => { if (debounceRef.current) clearTimeout(debounceRef.current); };
  }, [open, query, communityId]);

  const handlePick = useCallback((user) => {
    onSelect?.(user);
    setOpen(false);
    setQuery('');
    setResults([]);
  }, [onSelect]);

  const handleKeyDown = (e) => {
    if (!open || results.length === 0) return;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setActiveIndex((i) => (i + 1) % results.length);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActiveIndex((i) => (i - 1 + results.length) % results.length);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const pick = results[activeIndex];
      if (pick) handlePick(pick);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      setOpen(false);
    }
  };

  // Chip view when a user is already picked — clearing it restores
  // the input so the parent can reuse the same slot for a different
  // user without remounting.
  if (selected) {
    return (
      <div className="mention-picker mention-picker--chip" ref={rootRef}>
        {selected.avatar_path ? (
          <img src={selected.avatar_path} alt="" className="mention-picker__chip-avatar" />
        ) : (
          <span className="mention-picker__chip-avatar mention-picker__chip-avatar--initial">
            {(selected.username || '?').charAt(0).toUpperCase()}
          </span>
        )}
        <span className="mention-picker__chip-username">@{selected.username}</span>
        <button
          type="button"
          className="mention-picker__chip-clear"
          aria-label="Remove selected user"
          onClick={() => onClear?.()}
        >
          ×
        </button>
      </div>
    );
  }

  return (
    <div className="mention-picker" ref={rootRef}>
      <input
        ref={inputRef}
        type="text"
        className="mention-picker__input"
        placeholder={placeholder || (communityId ? 'Pick a community member…' : 'Pick a mutual…')}
        value={query}
        onChange={(e) => { setQuery(e.target.value); setOpen(true); }}
        onFocus={() => setOpen(true)}
        onKeyDown={handleKeyDown}
      />
      {open && results.length > 0 && (
        <ul className="mention-picker__panel" role="listbox" aria-label="User suggestions">
          {results.map((u, idx) => (
            <li
              key={u.id}
              role="option"
              aria-selected={idx === activeIndex}
              className={`mention-picker__item${idx === activeIndex ? ' mention-picker__item--active' : ''}`}
              onMouseDown={(e) => { e.preventDefault(); handlePick(u); }}
              onMouseEnter={() => setActiveIndex(idx)}
            >
              {u.avatar_path ? (
                <img src={u.avatar_path} alt="" className="mention-picker__avatar" />
              ) : (
                <span className="mention-picker__avatar mention-picker__avatar--initial">
                  {(u.username || '?').charAt(0).toUpperCase()}
                </span>
              )}
              <span className="mention-picker__username">@{u.username}</span>
            </li>
          ))}
        </ul>
      )}
      {open && results.length === 0 && query.length > 0 && (
        <div className="mention-picker__empty">
          {communityId ? 'No matching members.' : 'No matching mutuals.'}
        </div>
      )}
    </div>
  );
};

export default MentionPicker;
