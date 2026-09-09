import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { apiRequest, newIdempotencyKey } from './client';

beforeEach(() => vi.useRealTimers());
afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe('apiRequest', () => {
  it('sends same-site credentials, CSRF, and idempotency headers', async () => {
    document.cookie = 'nextstep_admin_csrf=csrf-value; path=/';
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ id: '1' }), { status: 200, headers: { 'Content-Type': 'application/json' } }));
    await apiRequest('/api/test', { method: 'POST', scope: 'admin', idempotencyKey: 'request-key', body: { value: 1 } });
    const [, options] = fetchMock.mock.calls[0]!;
    const headers = options?.headers as Headers;
    expect(options?.credentials).toBe('same-origin');
    expect(headers.get('X-CSRF-Token')).toBe('csrf-value');
    expect(headers.get('Idempotency-Key')).toBe('request-key');
  });

  it('preserves safe problem details without leaking arbitrary response text', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ error: { code: 'VERSION_CONFLICT', message: 'Reload', requestId: 'req-1', retryable: false } }), { status: 409, headers: { 'Content-Type': 'application/json' } }));
    await expect(apiRequest('/api/test')).rejects.toEqual(expect.objectContaining({ code: 'VERSION_CONFLICT', requestId: 'req-1' }));
  });

  it('creates compact unique idempotency keys', () => {
    expect(newIdempotencyKey('tenant')).toMatch(/^tenant-[0-9a-f-]{36}$/);
  });

  it('distinguishes an external cancellation from a request timeout', async () => {
    const external = new AbortController();
    vi.spyOn(globalThis, 'fetch').mockImplementation((_, options) => new Promise((_, reject) => {
      options?.signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true });
    }));

    const request = apiRequest('/api/test', { signal: external.signal });
    external.abort('route-changed');

    await expect(request).rejects.toEqual(expect.objectContaining({ code: 'CANCELLED', retryable: false }));
  });

  it('reports its own deadline as a retryable timeout', async () => {
    vi.useFakeTimers();
    vi.spyOn(globalThis, 'fetch').mockImplementation((_, options) => new Promise((_, reject) => {
      options?.signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true });
    }));

    const request = apiRequest('/api/test', { timeoutMs: 50, retryAttempts: 0 });
    const assertion = expect(request).rejects.toEqual(expect.objectContaining({ code: 'TIMEOUT', retryable: true }));
    await vi.advanceTimersByTimeAsync(50);

    await assertion;
  });

  it('retries a retryable GET but never creates a second mutation request', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: { code: 'TEMPORARY', message: 'Retry', requestId: 'req-1', retryable: true } }), { status: 503, headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'Content-Type': 'application/json' } }));

    await expect(apiRequest<{ ok: boolean }>('/api/test', { retryAttempts: 1, retryBaseDelayMs: 0 })).resolves.toEqual({ ok: true });
    expect(fetchMock).toHaveBeenCalledTimes(2);

    fetchMock.mockClear();
    fetchMock.mockResolvedValue(new Response(JSON.stringify({ error: { code: 'TEMPORARY', message: 'Retry', requestId: 'req-2', retryable: true } }), { status: 503, headers: { 'Content-Type': 'application/json' } }));
    await expect(apiRequest('/api/test', { method: 'POST', body: {}, retryAttempts: 2 })).rejects.toEqual(expect.objectContaining({ code: 'TEMPORARY' }));
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('announces an unauthorized response with its authentication scope', async () => {
    const listener = vi.fn();
    window.addEventListener('nextstep:unauthorized', listener);
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ error: { code: 'UNAUTHORIZED', message: 'Expired', requestId: 'req-auth', retryable: false } }), { status: 401, headers: { 'Content-Type': 'application/json' } }));

    await expect(apiRequest('/api/v1/admin/tenants')).rejects.toEqual(expect.objectContaining({ code: 'UNAUTHORIZED' }));
    expect(listener).toHaveBeenCalledOnce();
    expect((listener.mock.calls[0]![0] as CustomEvent).detail).toEqual({ scope: 'admin' });
    window.removeEventListener('nextstep:unauthorized', listener);
  });
});
