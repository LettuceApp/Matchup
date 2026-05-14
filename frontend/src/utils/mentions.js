/*
 * Mention utilities — shared between the autocomplete component
 * (which injects @username into a textarea on pick) and the comment
 * renderer (which turns @username into a clickable Link).
 *
 * The regex MUST stay aligned with the server-side mention parser at
 * api/controllers/mention_helpers.go's resolveMentionedUserIDs() —
 * if the two drift, the frontend will render mentions that don't
 * trigger backend notifications or vice versa. Same character class:
 * letters, digits, underscore; a non-word character (or start of
 * string) is required immediately before the `@` so emails like
 * `you@example` don't get mis-parsed as mentions.
 */

// Captures `@username` mentions only when preceded by start-of-string
// or a non-word character. The `(?:^|[^A-Za-z0-9_])` runs as a
// non-capturing prefix so split() / replace() callers see clean
// `@username` boundaries. Case-insensitive on the @ separator but the
// captured username preserves its casing for display.
export const MENTION_REGEX = /(^|[^A-Za-z0-9_])@([A-Za-z0-9_]+)/g;

/*
 * splitWithMentions — turn a body string into an array of parts so a
 * React renderer can map text segments to plain strings and mention
 * segments to <Link to="/users/<username>"> elements.
 *
 * Returns: [{type:'text', value:'…'}, {type:'mention', username:'…'}, ...]
 *
 * The leading-character prefix (e.g. the space before `@target`) is
 * preserved as a text segment so the output, rejoined, equals the
 * original string. Test:
 *   splitWithMentions('hi @bob and @alice!') =>
 *     [
 *       { type:'text', value:'hi ' },
 *       { type:'mention', username:'bob' },
 *       { type:'text', value:' and ' },
 *       { type:'mention', username:'alice' },
 *       { type:'text', value:'!' },
 *     ]
 */
export function splitWithMentions(body) {
  if (!body || typeof body !== 'string') return [{ type: 'text', value: body || '' }];

  const out = [];
  let lastIndex = 0;
  const re = new RegExp(MENTION_REGEX.source, 'g');
  let match;
  while ((match = re.exec(body)) !== null) {
    const [, prefix, username] = match;
    const mentionStart = match.index + prefix.length;
    // Everything up to (and including) the prefix character is text.
    if (mentionStart > lastIndex) {
      out.push({ type: 'text', value: body.slice(lastIndex, mentionStart) });
    }
    out.push({ type: 'mention', username });
    lastIndex = mentionStart + 1 /* @ */ + username.length;
  }
  if (lastIndex < body.length) {
    out.push({ type: 'text', value: body.slice(lastIndex) });
  }
  return out;
}

/*
 * extractActiveMentionQuery — given the current textarea value + the
 * caret position, return the partial username the user is currently
 * typing after an `@`, or null if the caret isn't inside a mention.
 *
 * The MentionAutocomplete component calls this on every keystroke
 * (and `selectionchange`) to decide whether to open the dropdown +
 * what to filter by. A `null` return closes the dropdown.
 *
 * Rules:
 *   - Look backward from `caret` for the nearest `@`.
 *   - If the chars between that `@` and `caret` are all word chars
 *     ([A-Za-z0-9_]) AND the char before the `@` is non-word (or
 *     start-of-string), return that substring (without the `@`).
 *   - Otherwise return null.
 *   - If the user has typed e.g. `@al ` (trailing space), the caret
 *     is past the space → the previous `@` no longer counts; null.
 */
export function extractActiveMentionQuery(value, caret) {
  if (!value || caret == null) return null;
  // Walk backward from caret-1 collecting word chars until we hit @.
  let i = caret - 1;
  while (i >= 0 && /[A-Za-z0-9_]/.test(value[i])) i -= 1;
  if (i < 0 || value[i] !== '@') return null;
  // Char immediately before `@` must be non-word OR string-start.
  if (i > 0 && /[A-Za-z0-9_]/.test(value[i - 1])) return null;
  return value.slice(i + 1, caret);
}

/*
 * replaceActiveMention — splice in the chosen username at the active
 * mention site. Returns { value, caret } so the textarea state +
 * cursor position can both update in one render.
 *
 * Appends a trailing space after the inserted username so the user
 * can keep typing without backspacing the autocomplete's marker.
 */
export function replaceActiveMention(value, caret, username) {
  if (!value || caret == null) return { value, caret };
  let i = caret - 1;
  while (i >= 0 && /[A-Za-z0-9_]/.test(value[i])) i -= 1;
  if (i < 0 || value[i] !== '@') return { value, caret };
  const before = value.slice(0, i); // up to (but not including) `@`
  const after = value.slice(caret); // from current caret onward
  const inserted = `@${username} `;
  return { value: before + inserted + after, caret: before.length + inserted.length };
}
