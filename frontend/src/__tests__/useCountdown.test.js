import { renderHook, act } from '@testing-library/react';
import useCountdown from '../hooks/useCountdown';

beforeEach(() => {
  jest.useFakeTimers();
});

afterEach(() => {
  jest.useRealTimers();
});

describe('useCountdown', () => {
  it('returns null remainingMs and isExpired=false when targetTime is null', () => {
    const { result } = renderHook(() => useCountdown(null));
    expect(result.current.remainingMs).toBeNull();
    expect(result.current.isExpired).toBe(false);
    expect(result.current.formatted).toBe('0:00:00');
  });

  it('returns 0 remainingMs and isExpired=true for a past targetTime', () => {
    const past = new Date(Date.now() - 3600 * 1000).toISOString();
    const { result } = renderHook(() => useCountdown(past));
    expect(result.current.remainingMs).toBe(0);
    expect(result.current.isExpired).toBe(true);
  });

  it('returns positive remainingMs and isExpired=false for a future targetTime', () => {
    const future = new Date(Date.now() + 3600 * 1000).toISOString();
    const { result } = renderHook(() => useCountdown(future));
    expect(result.current.remainingMs).toBeGreaterThan(0);
    expect(result.current.isExpired).toBe(false);
  });

  it('formats remaining time correctly', () => {
    // 1 hour + 5 minutes + 3 seconds from now
    const ms = (1 * 3600 + 5 * 60 + 3) * 1000;
    const future = new Date(Date.now() + ms).toISOString();
    const { result } = renderHook(() => useCountdown(future));
    expect(result.current.formatted).toBe('1:05:03');
  });

  it('counts down when the interval fires', () => {
    const future = new Date(Date.now() + 10000).toISOString();
    const { result } = renderHook(() => useCountdown(future));
    const initialMs = result.current.remainingMs;

    act(() => {
      jest.advanceTimersByTime(1000);
    });

    expect(result.current.remainingMs).toBeLessThan(initialMs);
  });

  it('transitions to isExpired after time elapses', () => {
    const future = new Date(Date.now() + 2000).toISOString();
    const { result } = renderHook(() => useCountdown(future));
    expect(result.current.isExpired).toBe(false);

    act(() => {
      jest.advanceTimersByTime(3000);
    });

    expect(result.current.isExpired).toBe(true);
    expect(result.current.remainingMs).toBe(0);
  });

  it('clears interval when targetTime becomes null', () => {
    const future = new Date(Date.now() + 5000).toISOString();
    const { result, rerender } = renderHook(({ t }) => useCountdown(t), {
      initialProps: { t: future },
    });
    expect(result.current.remainingMs).toBeGreaterThan(0);

    rerender({ t: null });
    expect(result.current.remainingMs).toBeNull();
    expect(result.current.isExpired).toBe(false);
  });
});
