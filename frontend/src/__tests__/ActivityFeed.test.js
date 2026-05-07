import React from 'react';
import { MemoryRouter } from 'react-router-dom';
import { render, screen, fireEvent } from '@testing-library/react';
import ActivityFeed from '../components/ActivityFeed';

const renderFeed = (props) =>
  render(
    <MemoryRouter>
      <ActivityFeed {...props} />
    </MemoryRouter>
  );

// Fixed "now" so relativeTime is deterministic across test runs. The
// ISO strings below sit at reliable offsets (5m / 2h / 1d) from this
// anchor.
const NOW = new Date('2026-04-21T12:00:00Z').getTime();
const isoMinutesAgo = (n) => new Date(NOW - n * 60_000).toISOString();

describe('ActivityFeed', () => {
  beforeEach(() => {
    jest.spyOn(Date, 'now').mockReturnValue(NOW);
  });
  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('renders the empty state when items is empty', () => {
    renderFeed({ items: [] });
    expect(screen.getByText(/Nothing yet/i)).toBeInTheDocument();
  });

  it('renders the loading state while loading', () => {
    renderFeed({ items: [], loading: true });
    expect(screen.getByText(/Loading activity/i)).toBeInTheDocument();
  });

  it('renders the error state when error is set', () => {
    renderFeed({ items: [], error: 'Activity unavailable.' });
    expect(screen.getByText(/Activity unavailable/i)).toBeInTheDocument();
  });

  it('shows a Retry button when onRetry is provided and fires it on click', () => {
    const onRetry = jest.fn();
    renderFeed({ items: [], error: 'Activity unavailable.', onRetry });
    const retry = screen.getByRole('button', { name: /retry/i });
    expect(retry).toBeInTheDocument();
    fireEvent.click(retry);
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it('does NOT show a Retry button when onRetry is not provided', () => {
    renderFeed({ items: [], error: 'Activity unavailable.' });
    expect(screen.queryByRole('button', { name: /retry/i })).toBeNull();
  });

  it('renders a vote_cast item with subject link + time', () => {
    const items = [{
      id: 'vote_cast:1',
      kind: 'vote_cast',
      occurred_at: isoMinutesAgo(5),
      subject_type: 'matchup',
      subject_id: 'uuid-matchup',
      subject_title: 'Best Rapper Alive',
      // author_username is needed for the real SPA link; backend now
      // includes it in the payload for every matchup-typed event.
      payload: { voted_item: 'Kendrick', author_username: 'cordell' },
    }];
    renderFeed({ items });
    expect(screen.getByText(/You voted/)).toBeInTheDocument();
    expect(screen.getByText('Kendrick')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Best Rapper Alive' })).toHaveAttribute(
      'href', '/users/cordell/matchup/uuid-matchup'
    );
    expect(screen.getByText(/5m ago/)).toBeInTheDocument();
  });

  it('renders a vote_win item with the trophy copy', () => {
    const items = [{
      id: 'vote_win:1',
      kind: 'vote_win',
      occurred_at: isoMinutesAgo(60),
      subject_type: 'matchup',
      subject_id: 'Kj3mN9xP',
      subject_title: 'Best Rapper Alive',
      payload: { voted_item: 'Kendrick' },
    }];
    renderFeed({ items });
    expect(screen.getByText(/You backed/)).toBeInTheDocument();
    // Emoji is inline in the message; search for the winner name.
    expect(screen.getByText('Kendrick')).toBeInTheDocument();
  });

  it('renders a new_follower item with @handle link', () => {
    const items = [{
      id: 'new_follower:9',
      kind: 'new_follower',
      occurred_at: isoMinutesAgo(10),
      actor_id: 'some-uuid',
      actor_username: 'fan',
      subject_type: 'user',
      subject_id: 'some-uuid',
      subject_title: 'fan',
      payload: {},
    }];
    renderFeed({ items });
    expect(screen.getByRole('link', { name: '@fan' })).toHaveAttribute('href', '/users/fan');
    expect(screen.getByText(/followed you/)).toBeInTheDocument();
  });

  it('renders a comment_received item with snippet + actor', () => {
    const items = [{
      id: 'comment_received:matchup:4',
      kind: 'comment_received',
      occurred_at: isoMinutesAgo(3),
      actor_id: 'abc',
      actor_username: 'drake_fan',
      subject_type: 'matchup',
      subject_id: 'Kj3mN9xP',
      subject_title: 'Best Rapper Alive',
      payload: { body: 'hot take' },
    }];
    renderFeed({ items });
    expect(screen.getByRole('link', { name: '@drake_fan' })).toBeInTheDocument();
    expect(screen.getByText(/commented on/)).toBeInTheDocument();
    expect(screen.getByText(/hot take/)).toBeInTheDocument();
  });

  it('adds the unread dot to items newer than lastSeenAt', () => {
    const items = [{
      id: 'like_received:matchup:1',
      kind: 'like_received',
      occurred_at: isoMinutesAgo(1), // after the cutoff
      actor_username: 'fan',
      subject_type: 'matchup',
      subject_id: 'Kj3mN9xP',
      subject_title: 'Best Rapper Alive',
      payload: {},
    }, {
      id: 'like_received:matchup:2',
      kind: 'like_received',
      occurred_at: isoMinutesAgo(999), // before the cutoff
      actor_username: 'fan2',
      subject_type: 'matchup',
      subject_id: 'Kj3mN9xP',
      subject_title: 'Best Rapper Alive',
      payload: {},
    }];
    const lastSeenAt = new Date(NOW - 10 * 60_000).toISOString();
    const { container } = renderFeed({ items, lastSeenAt });
    // Two items rendered, only the newer one has the unread class.
    // The "unread" state is a CSS-class-driven visual cue — there's no
    // accessible role/label that distinguishes it (yet), so a class-
    // selector query is the most direct assertion. testing-library's
    // no-container / no-node-access rules are intentionally relaxed
    // for this single case.
    /* eslint-disable testing-library/no-container, testing-library/no-node-access */
    const unread = container.querySelectorAll('.activity-item.is-unread');
    /* eslint-enable testing-library/no-container, testing-library/no-node-access */
    expect(unread).toHaveLength(1);
  });
});
