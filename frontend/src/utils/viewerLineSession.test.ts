import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '@/api';
import {
  clearViewerLineLoginRetry,
  exchangeViewerLineSession,
  lineLoginRedirectUri,
  viewerAuthenticationPlan,
  VIEWER_LINE_LOGIN_RETRY_KEY
} from './viewerLineSession';

function lineError(code = 'LINE_ID_TOKEN_INVALID') {
  return new ApiError(401, { code, message: 'LINE identity verification failed.', requestId: 'request-1', retryable: false });
}

function liffClient(overrides: Partial<{
  loggedIn: boolean;
  inClient: boolean;
  idToken: string | null;
}> = {}) {
  const state = {
    loggedIn: overrides.loggedIn ?? true,
    inClient: overrides.inClient ?? false,
    idToken: overrides.idToken === undefined ? 'opaque-line-id-token-that-is-long-enough' : overrides.idToken
  };
  return {
    init: vi.fn(async () => undefined),
    isLoggedIn: vi.fn(() => state.loggedIn),
    isInClient: vi.fn(() => state.inClient),
    getIDToken: vi.fn(() => state.idToken),
    login: vi.fn(),
    logout: vi.fn()
  };
}

describe('viewer LINE session recovery', () => {
  it('preserves invitation reference but removes transient OAuth parameters from the login redirect', () => {
    expect(lineLoginRedirectUri('https://dashboard.nextstep-soft.com/app/invite?ref=opaque-reference&code=secret-code&state=secret-state&liffClientId=2010662588')).toBe(
      'https://dashboard.nextstep-soft.com/app/invite?ref=opaque-reference'
    );
  });

  it('restores the canonical invitation URL when LINE returns through liff.state', () => {
    expect(lineLoginRedirectUri(
      'https://dashboard.nextstep-soft.com/app?liff.state=%2Finvite%3Fref%3Dopaque-reference&code=secret-code&state=secret-state',
      'opaque-reference'
    )).toBe('https://dashboard.nextstep-soft.com/app/invite?ref=opaque-reference');
  });

  it('requires LINE identity exchange for invitation links even when a viewer session may already exist', () => {
    expect(viewerAuthenticationPlan('opaque-invitation-reference')).toBe('LINE_IDENTITY_REQUIRED');
    expect(viewerAuthenticationPlan(undefined)).toBe('EXISTING_SESSION_FIRST');
  });

  it('logs in without exchanging when the external browser has no LINE session', async () => {
    const liff = liffClient({ loggedIn: false });
    const exchange = vi.fn();

    const result = await exchangeViewerLineSession({
      liff,
      liffId: '2010662588-skbKIlSU',
      redirectUri: 'https://dashboard.nextstep-soft.com/app/invite?ref=opaque',
      exchange,
      retryStorage: sessionStorage,
      now: () => 1_000
    });

    expect(result).toEqual({ kind: 'REDIRECTING' });
    expect(liff.login).toHaveBeenCalledOnce();
    expect(exchange).not.toHaveBeenCalled();
  });

  it('performs one automatic logout/login when LINE rejects a stale token in an external browser', async () => {
    const liff = liffClient();
    const exchange = vi.fn(async () => { throw lineError(); });

    const result = await exchangeViewerLineSession({
      liff,
      liffId: '2010662588-skbKIlSU',
      redirectUri: 'https://dashboard.nextstep-soft.com/app/invite?ref=opaque',
      exchange,
      retryStorage: sessionStorage,
      now: () => 2_000
    });

    expect(result).toEqual({ kind: 'REDIRECTING' });
    expect(liff.logout).toHaveBeenCalledOnce();
    expect(liff.login).toHaveBeenCalledOnce();
    expect(sessionStorage.getItem(VIEWER_LINE_LOGIN_RETRY_KEY)).toBe('2000');
  });

  it('does not create a redirect loop when the refreshed token is still invalid', async () => {
    sessionStorage.setItem(VIEWER_LINE_LOGIN_RETRY_KEY, '2000');
    const liff = liffClient();
    const exchange = vi.fn(async () => { throw lineError(); });

    await expect(exchangeViewerLineSession({
      liff,
      liffId: '2010662588-skbKIlSU',
      redirectUri: 'https://dashboard.nextstep-soft.com/app/invite?ref=opaque',
      exchange,
      retryStorage: sessionStorage,
      now: () => 2_500
    })).rejects.toMatchObject({ code: 'LINE_ID_TOKEN_INVALID' });

    expect(liff.logout).not.toHaveBeenCalled();
    expect(liff.login).not.toHaveBeenCalled();
  });

  it('does not call liff.login inside the LIFF browser', async () => {
    const liff = liffClient({ inClient: true });
    const exchange = vi.fn(async () => { throw lineError(); });

    await expect(exchangeViewerLineSession({
      liff,
      liffId: '2010662588-skbKIlSU',
      redirectUri: 'https://dashboard.nextstep-soft.com/app/invite?ref=opaque',
      exchange,
      retryStorage: sessionStorage,
      now: () => 3_000
    })).rejects.toMatchObject({ code: 'LINE_ID_TOKEN_INVALID' });

    expect(liff.logout).not.toHaveBeenCalled();
    expect(liff.login).not.toHaveBeenCalled();
  });

  it('clears the retry guard after a successful exchange', async () => {
    sessionStorage.setItem(VIEWER_LINE_LOGIN_RETRY_KEY, '2000');
    const liff = liffClient();
    const exchange = vi.fn(async () => ({ recipientId: 'recipient-1' }));

    const result = await exchangeViewerLineSession({
      liff,
      liffId: '2010662588-skbKIlSU',
      redirectUri: 'https://dashboard.nextstep-soft.com/app/invite?ref=opaque',
      exchange,
      retryStorage: sessionStorage,
      now: () => 4_000
    });

    expect(result).toEqual({ kind: 'EXCHANGED', value: { recipientId: 'recipient-1' } });
    expect(sessionStorage.getItem(VIEWER_LINE_LOGIN_RETRY_KEY)).toBeNull();
  });

  it('fails safely when LINE does not provide an ID token', async () => {
    const liff = liffClient({ idToken: null });

    await expect(exchangeViewerLineSession({
      liff,
      liffId: '2010662588-skbKIlSU',
      redirectUri: 'https://dashboard.nextstep-soft.com/app/invite?ref=opaque',
      exchange: vi.fn(),
      retryStorage: sessionStorage,
      now: () => 5_000
    })).rejects.toMatchObject({ code: 'LINE_ID_TOKEN_MISSING' });
  });
});

afterEach(() => {
  clearViewerLineLoginRetry(sessionStorage);
});
