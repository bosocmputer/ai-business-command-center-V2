import type { ProblemEnvelope } from './types';

type AuthScope = 'admin' | 'viewer';

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId: string;
  readonly retryable: boolean;
  readonly fieldErrors: ProblemEnvelope['error']['fieldErrors'];
  readonly retryAfterMs?: number;

  constructor(status: number, problem: ProblemEnvelope['error'], retryAfterMs?: number) {
    super(problem.message);
    this.name = 'ApiError';
    this.status = status;
    this.code = problem.code;
    this.requestId = problem.requestId;
    this.retryable = problem.retryable;
    this.fieldErrors = problem.fieldErrors;
    this.retryAfterMs = retryAfterMs;
  }
}

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
  body?: unknown;
  scope?: AuthScope;
  idempotencyKey?: string;
  timeoutMs?: number;
  signal?: AbortSignal;
  retryAttempts?: number;
  retryBaseDelayMs?: number;
}

const unsafeMethods = new Set(['POST', 'PUT', 'PATCH', 'DELETE']);

export async function apiRequest<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const method = options.method ?? 'GET';
  const retryAttempts = unsafeMethods.has(method) ? 0 : Math.max(0, options.retryAttempts ?? 1);
  let attempt = 0;

  while (true) {
    try {
      return await requestOnce<T>(path, method, options);
    } catch (error) {
      const apiError = normalizeError(error);
      if (apiError.code === 'CANCELLED' || attempt >= retryAttempts || !apiError.retryable) throw apiError;
      const delay = apiError.retryAfterMs ?? Math.max(0, options.retryBaseDelayMs ?? 250) * (2 ** attempt) + Math.floor(Math.random() * 100);
      attempt++;
      await waitForRetry(delay, options.signal);
    }
  }
}

async function requestOnce<T>(path: string, method: NonNullable<RequestOptions['method']> | 'GET', options: RequestOptions): Promise<T> {
  if (options.signal?.aborted) throw cancelledError();
  const controller = new AbortController();
  let timedOut = false;
  const timeout = window.setTimeout(() => {
    timedOut = true;
    controller.abort('timeout');
  }, options.timeoutMs ?? 20_000);
  const abort = () => controller.abort(options.signal?.reason);
  options.signal?.addEventListener('abort', abort, { once: true });
  const headers = new Headers({ Accept: 'application/json' });
  if (options.body !== undefined) headers.set('Content-Type', 'application/json');
  if (options.idempotencyKey) headers.set('Idempotency-Key', options.idempotencyKey);
  if (options.scope && unsafeMethods.has(method)) {
    const csrf = readCookie(options.scope === 'admin' ? 'nextstep_admin_csrf' : 'nextstep_viewer_csrf');
    if (csrf) headers.set('X-CSRF-Token', csrf);
  }

  try {
    const response = await fetch(path, {
      method,
      headers,
      credentials: 'same-origin',
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
      signal: controller.signal
    });
    if (response.status === 204) return undefined as T;
    const payload = await parseJSON(response);
    if (!response.ok) {
      const retryAfterMs = parseRetryAfter(response.headers.get('Retry-After'));
      const error = isProblem(payload)
        ? new ApiError(response.status, payload.error, retryAfterMs)
        : new ApiError(response.status, {
          code: response.status === 401 ? 'UNAUTHORIZED' : 'HTTP_ERROR',
          message: response.status === 401 ? 'Session หมดอายุ กรุณาเข้าสู่ระบบใหม่' : 'ระบบตอบกลับไม่สมบูรณ์ กรุณาลองใหม่',
          requestId: response.headers.get('X-Request-ID') ?? '',
          retryable: response.status === 429 || response.status >= 500
        }, retryAfterMs);
      const authScope = options.scope ?? inferAuthScope(path);
      if (response.status === 401 && authScope) announceUnauthorized(authScope);
      throw error;
    }
    return payload as T;
  } catch (error) {
    if (error instanceof ApiError) throw error;
    if (options.signal?.aborted) throw cancelledError();
    if (timedOut) throw new ApiError(0, { code: 'TIMEOUT', message: 'การเชื่อมต่อใช้เวลานานเกินไป กรุณาลองใหม่', requestId: '', retryable: true });
    throw new ApiError(0, { code: 'NETWORK_ERROR', message: 'ไม่สามารถเชื่อมต่อระบบได้ กรุณาตรวจสอบอินเทอร์เน็ตแล้วลองใหม่', requestId: '', retryable: true });
  } finally {
    window.clearTimeout(timeout);
    options.signal?.removeEventListener('abort', abort);
  }
}

function normalizeError(error: unknown): ApiError {
  return error instanceof ApiError
    ? error
    : new ApiError(0, { code: 'NETWORK_ERROR', message: 'ไม่สามารถเชื่อมต่อระบบได้', requestId: '', retryable: true });
}

function cancelledError(): ApiError {
  return new ApiError(0, { code: 'CANCELLED', message: 'ยกเลิกคำขอแล้ว', requestId: '', retryable: false });
}

function waitForRetry(delayMs: number, signal?: AbortSignal): Promise<void> {
  if (signal?.aborted) return Promise.reject(cancelledError());
  if (delayMs <= 0) return Promise.resolve();
  return new Promise((resolve, reject) => {
    const timeout = window.setTimeout(() => {
      signal?.removeEventListener('abort', abort);
      resolve();
    }, delayMs);
    const abort = () => {
      window.clearTimeout(timeout);
      reject(cancelledError());
    };
    signal?.addEventListener('abort', abort, { once: true });
  });
}

function parseRetryAfter(value: string | null): number | undefined {
  if (!value) return undefined;
  const seconds = Number(value);
  if (Number.isFinite(seconds)) return Math.max(0, seconds * 1000);
  const date = Date.parse(value);
  return Number.isNaN(date) ? undefined : Math.max(0, date - Date.now());
}

function announceUnauthorized(scope: AuthScope): void {
  window.dispatchEvent(new CustomEvent('nextstep:unauthorized', { detail: { scope } }));
}

function inferAuthScope(path: string): AuthScope | undefined {
  if (path.startsWith('/api/v1/admin/')) return 'admin';
  if (path.startsWith('/api/v1/viewer/') && !path.includes('/viewer/line/session')) return 'viewer';
  return undefined;
}

export function newIdempotencyKey(prefix: string): string {
  return `${prefix}-${crypto.randomUUID()}`;
}

export function queryString(values: Record<string, string | number | undefined | null>): string {
  const query = new URLSearchParams();
  Object.entries(values).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') query.set(key, String(value));
  });
  const encoded = query.toString();
  return encoded ? `?${encoded}` : '';
}

function readCookie(name: string): string {
  const prefix = `${encodeURIComponent(name)}=`;
  const match = document.cookie.split('; ').find((item) => item.startsWith(prefix));
  return match ? decodeURIComponent(match.slice(prefix.length)) : '';
}

async function parseJSON(response: Response): Promise<unknown> {
  const contentType = response.headers.get('Content-Type') ?? '';
  if (!contentType.includes('application/json')) return null;
  try {
    return await response.json();
  } catch {
    return null;
  }
}

function isProblem(value: unknown): value is ProblemEnvelope {
  if (!value || typeof value !== 'object' || !('error' in value)) return false;
  const error = (value as { error?: unknown }).error;
  return !!error && typeof error === 'object' && typeof (error as { code?: unknown }).code === 'string';
}
