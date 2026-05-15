import React, { useRef } from 'react';
import MentionAutocomplete from './MentionAutocomplete';

/*
 * MentionableInput — a single-line <input> with @-mention autocomplete
 * anchored to it. Drop-in replacement for a controlled <input> when
 * the parent wants the contents to support @-mentions for free.
 *
 * Why not just paste MentionAutocomplete next to each input in
 * CreateMatchup? Each item row needs its OWN textarea ref so the
 * autocomplete listens to the right element. Doing this with a Map
 * keyed by index gets messy fast (refs are tricky in render loops).
 * Wrapping the (input + autocomplete + position:relative parent) in
 * one component gives each row its own self-contained ref + scope.
 *
 * Props:
 *   value: string (controlled)
 *   onChange: (next: string) => void
 *   communityId?: string — when set, autocomplete suggests community
 *     members (overrides source)
 *   source: 'mutuals' | 'all' — broader suggestion pool for matchup
 *     items where we want maximum shareability
 *   enableMe: boolean — prepend an "@me" synthetic row that the
 *     backend resolves to the viewer on submit
 *   inputClassName: pass-through className for the underlying <input>
 *   plus any other input-level props (placeholder, onBlur, etc.).
 */
const MentionableInput = React.forwardRef(({
  value,
  onChange,
  communityId,
  source = 'all',
  enableMe = false,
  inputClassName,
  ...rest
}, forwardedRef) => {
  const localRef = useRef(null);
  // Combine the local ref with any forwarded ref so the parent can
  // still imperatively focus / select if it wants. The autocomplete
  // listens on the local ref (the one element it cares about).
  const setRefs = (el) => {
    localRef.current = el;
    if (typeof forwardedRef === 'function') forwardedRef(el);
    else if (forwardedRef) forwardedRef.current = el;
  };

  return (
    // position: relative so MentionAutocomplete's absolute dropdown
    // anchors against this wrapper, not the page body. Width 100%
    // so the input fills whatever flex slot the parent gave it.
    <div className="mentionable-input">
      <input
        type="text"
        ref={setRefs}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={inputClassName}
        {...rest}
      />
      <MentionAutocomplete
        value={value}
        onChange={(next) => onChange(next)}
        textareaRef={localRef}
        communityId={communityId}
        source={source}
        enableMe={enableMe}
      />
    </div>
  );
});

MentionableInput.displayName = 'MentionableInput';

export default MentionableInput;
