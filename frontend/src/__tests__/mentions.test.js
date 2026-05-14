/*
 * Regression tests for the mention utilities.
 *
 * The regex MUST stay aligned with the server-side mention parser in
 * api/controllers/mention_helpers.go's resolveMentionedUserIDs() — if
 * the two drift, the frontend renders mentions that don't trigger
 * backend notifications, or vice versa. These tests pin the
 * behaviour we expect on both sides.
 */

import {
  splitWithMentions,
  extractActiveMentionQuery,
  replaceActiveMention,
} from '../utils/mentions';

describe('splitWithMentions', () => {
  it('returns a single text part for empty input', () => {
    expect(splitWithMentions('')).toEqual([{ type: 'text', value: '' }]);
  });

  it('returns a single text part when no mentions present', () => {
    expect(splitWithMentions('just a comment')).toEqual([
      { type: 'text', value: 'just a comment' },
    ]);
  });

  it('extracts a leading mention', () => {
    expect(splitWithMentions('@bob hello')).toEqual([
      { type: 'mention', username: 'bob' },
      { type: 'text', value: ' hello' },
    ]);
  });

  it('extracts mid-string mentions and preserves spacing', () => {
    expect(splitWithMentions('hi @bob and @alice!')).toEqual([
      { type: 'text', value: 'hi ' },
      { type: 'mention', username: 'bob' },
      { type: 'text', value: ' and ' },
      { type: 'mention', username: 'alice' },
      { type: 'text', value: '!' },
    ]);
  });

  it('does NOT mention inside emails (word char before @)', () => {
    expect(splitWithMentions('email me at you@example')).toEqual([
      { type: 'text', value: 'email me at you@example' },
    ]);
  });

  it('accepts underscores + digits in usernames', () => {
    expect(splitWithMentions('shout @cool_user_42 nice')).toEqual([
      { type: 'text', value: 'shout ' },
      { type: 'mention', username: 'cool_user_42' },
      { type: 'text', value: ' nice' },
    ]);
  });
});

describe('extractActiveMentionQuery', () => {
  it('returns the partial when caret sits in an open mention', () => {
    expect(extractActiveMentionQuery('hi @al', 6)).toBe('al');
  });

  it('returns empty string immediately after the @', () => {
    expect(extractActiveMentionQuery('hi @', 4)).toBe('');
  });

  it('returns null when caret is past a closed mention (space after)', () => {
    expect(extractActiveMentionQuery('hi @al ', 7)).toBe(null);
  });

  it('returns null when the @ is preceded by a word char (email)', () => {
    expect(extractActiveMentionQuery('foo@al', 6)).toBe(null);
  });

  it('returns null when caret is in non-word context', () => {
    expect(extractActiveMentionQuery('hi there', 5)).toBe(null);
  });
});

describe('replaceActiveMention', () => {
  it('replaces the active partial + adds trailing space', () => {
    const { value, caret } = replaceActiveMention('hi @al', 6, 'alice');
    expect(value).toBe('hi @alice ');
    expect(caret).toBe(10);
  });

  it('handles caret immediately after @', () => {
    const { value } = replaceActiveMention('hi @', 4, 'alice');
    expect(value).toBe('hi @alice ');
  });

  it('preserves text after the caret', () => {
    const { value, caret } = replaceActiveMention('hi @al!', 6, 'alice');
    expect(value).toBe('hi @alice !');
    expect(caret).toBe(10);
  });

  it('returns input unchanged when no active mention', () => {
    const out = replaceActiveMention('hi there', 5, 'alice');
    expect(out.value).toBe('hi there');
    expect(out.caret).toBe(5);
  });
});
