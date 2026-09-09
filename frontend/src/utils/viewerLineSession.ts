import { ApiError } from '@/api';

export const VIEWER_LINE_LOGIN_RETRY_KEY = 'nextstep.viewer.line-login-retry.v1';
const retryWindowMs = 5 * 60 * 1000;
const lineOAuthTransientQueryKeys = ['code', 'state', 'liffClientId', 'liffRedirectUri'];

export type ViewerAuthenticationPlan = 'LINE_IDENTITY_REQUIRED' | 'EXISTING_SESSION_FIRST';

export interface ViewerLiffClient {
  init(config: { liffId: string }): Promise<void>;
  isLoggedIn(): boolean;
  isInClient(): boolean;
  getIDToken(): string | null;
  login(config: { redirectUri: string }): void;
  logout(): void;
}

interface RetryStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

export class ViewerLineSessionError extends Error {
  readonly code: 'LINE_ID_TOKEN_MISSING';

  constructor(code: 'LINE_ID_TOKEN_MISSING') {
    super(code);
    this.name = 'ViewerLineSessionError';
    this.code = code;
  }
}

export function viewerAuthenticationPlan(invitationReference?: string): ViewerAuthenticationPlan {
  return invitationReference ? 'LINE_IDENTITY_REQUIRED' : 'EXISTING_SESSION_FIRST';
}

export function lineLoginRedirectUri(currentHref: string, invitationReference?: string): string {
  const redirect = new URL(currentHref);
  if (invitationReference) {
    redirect.pathname = '/app/invite';
    redirect.search = '';
    redirect.hash = '';
    redirect.searchParams.set('ref', invitationReference);
    return redirect.toString();
  }
  lineOAuthTransientQueryKeys.forEach((key) => redirect.searchParams.delete(key));
  return redirect.toString();
}

export async function exchangeViewerLineSession<T>(input: {
  liff: ViewerLiffClient;
  liffId: string;
  redirectUri: string;
  exchange: (idToken: string) => Promise<T>;
  retryStorage?: RetryStorage;
  now?: () => number;
}): Promise<{ kind: 'REDIRECTING' } | { kind: 'EXCHANGED'; value: T }> {
  await input.liff.init({ liffId: input.liffId });
  if (!input.liff.isLoggedIn()) {
    input.liff.login({ redirectUri: input.redirectUri });
    return { kind: 'REDIRECTING' };
  }

  const idToken = input.liff.getIDToken();
  if (!idToken) throw new ViewerLineSessionError('LINE_ID_TOKEN_MISSING');

  try {
    const value = await input.exchange(idToken);
    clearViewerLineLoginRetry(input.retryStorage);
    return { kind: 'EXCHANGED', value };
  } catch (cause) {
    const shouldRecover = cause instanceof ApiError
      && cause.code === 'LINE_ID_TOKEN_INVALID'
      && !input.liff.isInClient()
      && claimViewerLineLoginRetry(input.retryStorage, input.now?.() ?? Date.now());
    if (!shouldRecover) throw cause;

    input.liff.logout();
    input.liff.login({ redirectUri: input.redirectUri });
    return { kind: 'REDIRECTING' };
  }
}

export function clearViewerLineLoginRetry(storage?: RetryStorage): void {
  if (!storage) return;
  try { storage.removeItem(VIEWER_LINE_LOGIN_RETRY_KEY); }
  catch { /* Storage can be unavailable in privacy-restricted browsers. */ }
}

function claimViewerLineLoginRetry(storage: RetryStorage | undefined, now: number): boolean {
  if (!storage) return false;
  try {
    const previous = Number(storage.getItem(VIEWER_LINE_LOGIN_RETRY_KEY));
    if (Number.isFinite(previous) && previous > 0 && now >= previous && now - previous < retryWindowMs) return false;
    storage.setItem(VIEWER_LINE_LOGIN_RETRY_KEY, String(now));
    return true;
  } catch {
    return false;
  }
}
