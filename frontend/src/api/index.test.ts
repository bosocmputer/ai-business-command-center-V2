import { afterEach, describe, expect, it, vi } from 'vitest';
import { adminApi, viewerApi } from './index';

afterEach(() => vi.restoreAllMocks());

describe('adminApi recipients', () => {
  it('reissues a pending invitation with CSRF and a stable idempotency key', async () => {
    document.cookie = 'nextstep_admin_csrf=csrf-value; path=/';
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ id: 'recipient-1', status: 'PENDING', invitationUrl: 'https://dashboard.nextstep-soft.com/app/invite?ref=new' }), { status: 200, headers: { 'Content-Type': 'application/json' } }));

    await adminApi.reissueRecipientInvitation('tenant-1', 'recipient-1', 'recipient-reissue-stable');

    const [url, options] = fetchMock.mock.calls[0]!;
    expect(url).toBe('/api/v1/admin/tenants/tenant-1/recipients/recipient-1/invitation');
    expect(options?.method).toBe('POST');
    expect((options?.headers as Headers).get('X-CSRF-Token')).toBe('csrf-value');
    expect((options?.headers as Headers).get('Idempotency-Key')).toBe('recipient-reissue-stable');
  });

  it('revokes a recipient within the selected tenant with an authenticated DELETE', async () => {
    document.cookie = 'nextstep_admin_csrf=csrf-value; path=/';
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(null, { status: 204 }));

    await adminApi.revokeRecipient('tenant-1', 'recipient-1');

    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, options] = fetchMock.mock.calls[0]!;
    expect(url).toBe('/api/v1/admin/tenants/tenant-1/recipients/recipient-1');
    expect(options?.method).toBe('DELETE');
    expect((options?.headers as Headers).get('X-CSRF-Token')).toBe('csrf-value');
  });

  it('queries permission dependencies without mutating recipient state', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ recipientId: 'recipient-1', permissionsVersion: 2, items: [] }), { status: 200, headers: { 'Content-Type': 'application/json' } }));
    await adminApi.permissionDependencies('tenant-1', 'recipient-1');
    const [url, options] = fetchMock.mock.calls[0]!;
    expect(url).toBe('/api/v1/admin/tenants/tenant-1/recipients/recipient-1/permission-dependencies');
    expect(options?.method ?? 'GET').toBe('GET');
  });

  it('queries an exact recipient page with Admin CSRF protection', async () => {
    document.cookie = 'nextstep_admin_csrf=csrf-value; path=/';
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ data: [], page: 0, pageSize: 25, total: 0, hasMore: false }), { status: 200, headers: { 'Content-Type': 'application/json' } }));
    await adminApi.queryRecipients('tenant-1', { search: 'ผู้บริหาร', status: 'ACTIVE', permissionState: 'WITH_REPORTS', page: 0, pageSize: 25 });
    const [url, options] = fetchMock.mock.calls[0]!;
    expect(url).toBe('/api/v1/admin/tenants/tenant-1/recipients/query');
    expect(options?.method).toBe('POST');
    expect((options?.headers as Headers).get('X-CSRF-Token')).toBe('csrf-value');
    expect(options?.body).toBe(JSON.stringify({ search: 'ผู้บริหาร', status: 'ACTIVE', permissionState: 'WITH_REPORTS', page: 0, pageSize: 25 }));
  });

  it('queries schedule recipient eligibility through the read-only body endpoint', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ data: [], selected: [], page: 0, pageSize: 25, total: 0, hasMore: false }), { status: 200, headers: { 'Content-Type': 'application/json' } }));
    await adminApi.scheduleRecipientOptions('tenant-1', { reportKeys: ['sales_goods_services'], selectedRecipientIds: [], search: '', page: 0, pageSize: 25 });
    const [url, options] = fetchMock.mock.calls[0]!;
    expect(url).toBe('/api/v1/admin/tenants/tenant-1/schedule-recipient-options/query');
    expect(options?.method).toBe('POST');
  });
});

describe('adminApi tenants', () => {
  it('archives a tenant with its optimistic version and CSRF protection', async () => {
    document.cookie = 'nextstep_admin_csrf=csrf-value; path=/';
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(null, { status: 204 }));

    await adminApi.archiveTenant('tenant-1', 4);

    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, options] = fetchMock.mock.calls[0]!;
    expect(url).toBe('/api/v1/admin/tenants/tenant-1?version=4');
    expect(options?.method).toBe('DELETE');
    expect((options?.headers as Headers).get('X-CSRF-Token')).toBe('csrf-value');
  });
});

describe('adminApi operational incidents', () => {
  it('loads only the requested safe incident filters', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ data: [], hasMore: false }), { status: 200, headers: { 'Content-Type': 'application/json' } }));

    await adminApi.incidents({ status: 'OPEN', severity: 'P1', pageSize: 100 });

    const [url, options] = fetchMock.mock.calls[0]!;
    expect(url).toBe('/api/v1/admin/operational-incidents?status=OPEN&severity=P1&pageSize=100&scope=ACTIVE');
    expect(options?.method ?? 'GET').toBe('GET');
  });

  it('loads bounded incident occurrences from the Admin-only detail endpoint', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ data: [], page: { hasMore: false } }), { status: 200, headers: { 'Content-Type': 'application/json' } }));
    await adminApi.incidentOccurrences('11111111-1111-4111-8111-111111111111', 'cursor-value');
    const [url] = fetchMock.mock.calls[0]!;
    expect(url).toBe('/api/v1/admin/operational-incidents/11111111-1111-4111-8111-111111111111/occurrences?cursor=cursor-value&pageSize=25');
  });

  it('loads one persisted diagnosis without starting a JavaWS test', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      assessment: {
        problemArea: 'CUSTOMER_JAVA_WS', investigationOwner: 'CUSTOMER_IT', loadSignal: 'INSUFFICIENT_EVIDENCE',
        summaryTh: 'คำตอบไม่สมบูรณ์', problemAreaTh: 'Java Web Service', ownerTh: 'ผู้ดูแลลูกค้า',
        loadSignalTh: 'หลักฐานยังไม่พอ', customerActionTh: 'ตรวจ Java Web Service'
      },
      baseline: { sampleCount: 0 }
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }));

    await adminApi.incidentDiagnosis('incident-id', 'occurrence-id');

    const [url, options] = fetchMock.mock.calls[0]!;
    expect(url).toBe('/api/v1/admin/operational-incidents/incident-id/occurrences/occurrence-id/diagnosis');
    expect(options?.method ?? 'GET').toBe('GET');
    expect(String(url)).not.toMatch(/sml-connection\/test|report-runs/);
  });

  it('acknowledges by optimistic version with admin CSRF protection', async () => {
    document.cookie = 'nextstep_admin_csrf=csrf-value; path=/';
    const incident = {
      id: 'incident-1', alertRef: 'NST-ABC123DEF456', status: 'OPEN', severity: 'P1', rootCause: 'PLATFORM',
      incidentType: 'WORKER_HEARTBEAT_MISSING', occurrenceCount: 1, affectedCount: 1,
      activeAffectedCount: 1, observationMode: 'CONTINUOUS', subjectType: 'CONTAINER',
      firstSeenAt: '2026-07-16T01:00:00Z', lastSeenAt: '2026-07-16T01:00:00Z', version: 4,
      presentation: { titleTh: 'ระบบประมวลผลงานไม่ตอบสนอง', summaryTh: 'ไม่พบสัญญาณการทำงาน', stageTh: 'ตรวจระบบ', nextActionsTh: ['ตรวจสอบ Worker'] as string[] },
      statusPresentation: { state: 'ACTIVE_PROBLEM', headlineTh: 'ระบบประมวลผลงานไม่ตอบสนอง', statusSummaryTh: 'ควรเปิดรายละเอียดและตรวจสอบ', actionRequired: true },
      isDownstream: false
    } as const;
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ ...incident, status: 'ACKNOWLEDGED', version: 5 }), { status: 200, headers: { 'Content-Type': 'application/json' } }));

    await adminApi.acknowledgeIncident(incident);

    const [url, options] = fetchMock.mock.calls[0]!;
    expect(url).toBe('/api/v1/admin/operational-incidents/incident-1/acknowledge');
    expect(options?.method).toBe('POST');
    expect((options?.headers as Headers).get('X-CSRF-Token')).toBe('csrf-value');
    expect(options?.body).toBe(JSON.stringify({ version: 4 }));
  });
});

describe('viewerApi delivery contexts', () => {
  it('resolves a reference once through a CSRF-protected body without putting the token in the URL', async () => {
    document.cookie = 'nextstep_viewer_csrf=viewer-csrf; path=/';
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      deliveryId: '11111111-1111-4111-8111-111111111111', tenantId: '88bfcb51-73fe-469a-964a-675e6386c644', reports: []
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }));
    const reference = 'r'.repeat(32);

    await viewerApi.resolveDeliveryContext(reference, '88bfcb51-73fe-469a-964a-675e6386c644');

    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, options] = fetchMock.mock.calls[0]!;
    expect(url).toBe('/api/v1/viewer/delivery-contexts');
    expect(String(url)).not.toContain(reference);
    expect(options?.method).toBe('POST');
    expect((options?.headers as Headers).get('X-CSRF-Token')).toBe('viewer-csrf');
    expect(options?.body).toContain(reference);
  });

  it('loads a clean delivery report snapshot without calling a run or revalidation endpoint', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({}), { status: 200, headers: { 'Content-Type': 'application/json' } }));

    await viewerApi.deliveryReport('tenant-id', 'delivery-id', 'sales_goods_services');

    const [url, options] = fetchMock.mock.calls[0]!;
    expect(url).toBe('/api/v1/viewer/tenants/tenant-id/deliveries/delivery-id/reports/sales_goods_services');
    expect(options?.method ?? 'GET').toBe('GET');
    expect(String(url)).not.toMatch(/runs|revalidations/);
  });

  it('filters only stored report rows with Viewer CSRF protection', async () => {
    document.cookie = 'nextstep_viewer_csrf=viewer-csrf; path=/';
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ runId: 'run-id', columns: ['ic_code'], data: [], page: 0, pageSize: 25, total: 0 }), { status: 200, headers: { 'Content-Type': 'application/json' } }));

    await viewerApi.queryRows('tenant-id', 'stock_balance', 'run-id', { filters: [{ columnKey: 'ic_code', operator: 'CONTAINS', value: '001' }], page: 0, pageSize: 25 });

    const [url, options] = fetchMock.mock.calls[0]!;
    expect(url).toBe('/api/v1/viewer/tenants/tenant-id/reports/stock_balance/runs/run-id/rows/query');
    expect(options?.method).toBe('POST');
    expect((options?.headers as Headers).get('X-CSRF-Token')).toBe('viewer-csrf');
    expect(String(url)).not.toContain('revalidations');
  });
});
