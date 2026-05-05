import { performRefresh, signOutLocally } from '../services/api';

// Stub axios by importing the module after jest.mock so API.post is a
// capture-only mock. The interceptor's live-HTTP flow is covered by
// the server-side integration tests + a manual live-verify walk-
// through; this suite focuses on the pure-JS bits of api.js that run
// without a network: storage reads/writes + exported helpers.

jest.mock('axios', () => {
  const instance = {
    interceptors: {
      request: { use: jest.fn() },
      response: { use: jest.fn() },
    },
    defaults: { headers: {} },
    post: jest.fn(),
    request: jest.fn(),
  };
  return {
    __esModule: true,
    default: {
      create: () => instance,
    },
    create: () => instance,
  };
});

describe('signOutLocally', () => {
  beforeEach(() => {
    localStorage.setItem('token', 'old-access');
    localStorage.setItem('refresh_token', 'old-refresh');
    localStorage.setItem('userId', '42');
    localStorage.setItem('username', 'alice');
    localStorage.setItem('isAdmin', 'true');
  });

  it('clears every auth-related localStorage key', () => {
    signOutLocally();
    expect(localStorage.getItem('token')).toBeNull();
    expect(localStorage.getItem('refresh_token')).toBeNull();
    expect(localStorage.getItem('userId')).toBeNull();
    expect(localStorage.getItem('username')).toBeNull();
    expect(localStorage.getItem('isAdmin')).toBeNull();
  });
});

describe('performRefresh', () => {
  let axios;
  let instance;

  beforeEach(() => {
    axios = require('axios');
    instance = axios.create();
    instance.post.mockReset();
    localStorage.clear();
  });

  it('throws when no refresh_token is stored', async () => {
    await expect(performRefresh()).rejects.toThrow(/no refresh token/);
  });

  it('stores the new access + refresh token and returns the new access', async () => {
    localStorage.setItem('refresh_token', 'old-refresh');
    instance.post.mockResolvedValueOnce({
      data: {
        token: 'new-access',
        refresh_token: 'new-refresh',
        access_token_expires_in: 900,
      },
    });

    const got = await performRefresh();

    expect(got).toBe('new-access');
    expect(localStorage.getItem('token')).toBe('new-access');
    expect(localStorage.getItem('refresh_token')).toBe('new-refresh');
    expect(instance.defaults.headers.Authorization).toBe('Bearer new-access');
  });

  it('throws on malformed refresh responses (missing tokens)', async () => {
    localStorage.setItem('refresh_token', 'old-refresh');
    instance.post.mockResolvedValueOnce({ data: {} });

    await expect(performRefresh()).rejects.toThrow(/malformed refresh response/);
    // Storage untouched — we don't partial-update.
    expect(localStorage.getItem('refresh_token')).toBe('old-refresh');
    expect(localStorage.getItem('token')).toBeNull();
  });
});
