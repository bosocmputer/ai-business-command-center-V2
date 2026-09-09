import { expect, type Page, test } from '@playwright/test';

const api = '/api/v1';
const tenantId = '11111111-1111-4111-8111-111111111111';
const adminReportCatalog = {
  data: [
    ['sales_goods_services', 'รายงานขายสินค้าและบริการ', 'SALES', 'ขาย', 'DATE_RANGE'],
    ['purchase_goods_payables', 'รายงานซื้อสินค้าและตั้งหนี้', 'PURCHASE', 'ซื้อ', 'DATE_RANGE'],
    ['gross_profit_by_product', 'กำไรขั้นต้นตามสินค้า', 'GROSS_PROFIT', 'กำไรขั้นต้น', 'DATE_RANGE'],
    ['gross_profit_by_ar_customer', 'กำไรขั้นต้นตามลูกหนี้', 'GROSS_PROFIT', 'กำไรขั้นต้น', 'DATE_RANGE'],
    ['stock_balance', 'รายงานสต็อกคงเหลือ', 'INVENTORY', 'สินค้าคงคลัง', 'AS_OF_DATE'],
    ['stock_reorder', 'รายงานสินค้าถึงจุดสั่งซื้อ', 'INVENTORY', 'สินค้าคงคลัง', 'CURRENT_ONLY'],
    ['ar_customer_movement', 'รายงานความเคลื่อนไหวลูกหนี้', 'AR', 'ลูกหนี้', 'AS_OF_DATE'],
    ['ar_debt_receipt', 'รายงานรับชำระหนี้', 'AR', 'ลูกหนี้', 'DATE_RANGE'],
    ['cash_bank_receipts', 'รายงานรับเงิน', 'CASH_BANK', 'เงินสดและธนาคาร', 'DATE_RANGE'],
    ['cash_bank_payments', 'รายงานจ่ายเงิน', 'CASH_BANK', 'เงินสดและธนาคาร', 'DATE_RANGE']
  ].map(([reportKey, label, category, categoryLabel, periodMode]) => ({ reportKey, label, category, categoryLabel, periodMode, version: '1.0.0', status: 'ACTIVE' })),
  limits: { maxScheduleReports: 10, maxFlexPayloadBytes: 30720 }
};

function json(data: unknown, status = 200) {
  return { status, contentType: 'application/json', body: JSON.stringify(data) };
}

function tableResult<T>(data: T[], page = 0, pageSize = 25, total = data.length) {
  return { data, page, pageSize, total, totalPages: total ? Math.ceil(total / pageSize) : 0 };
}

function isTableQuery(url: string) {
  return new URL(url).pathname.endsWith('/query');
}

function unauthorized() {
  return json({ error: { code: 'UNAUTHORIZED', message: 'Authentication required.', requestId: 'e2e', retryable: false } }, 401);
}

function captureUnexpectedConsoleErrors(page: Page, allowedStatusCodes: number[] = []) {
  const errors: string[] = [];
  page.on('console', (message) => {
    const text = message.text();
    const allowedStatus = allowedStatusCodes.some((status) => text.includes(`status of ${status}`));
    if (message.type() === 'error' && !text.includes('status of 401 (Unauthorized)') && !allowedStatus) errors.push(text);
  });
  return errors;
}

test.beforeEach(async ({ page }) => {
  await page.route(`**${api}/admin/operational-incidents**`, (route) => route.fulfill(json(isTableQuery(route.request().url()) ? tableResult([]) : { data: [], page: { hasMore: false } })));
  await page.route(`**${api}/admin/tenants/*/recipients/*/permission-dependencies`, (route) => route.fulfill(json({
    recipientId: '33333333-3333-4333-8333-333333333333', permissionsVersion: 1, items: []
  })));
  await page.route(`**${api}/admin/tenants/*/dashboard-refresh-policy`, (route) => route.fulfill(json({
    tenantId, fastIntervalMinutes: 5, standardIntervalMinutes: 15, heavyIntervalMinutes: 30, version: 0
  })));
  await page.route(`**${api}/viewer/tenants/*/reports/*/revalidations`, (route) => route.fulfill(json({
    disposition: 'DISABLED', legacyFallback: true
  })));
});

async function mockAdminLogin(page: Page) {
  let authenticated = false;
  const session = { username: 'superadmin', expiresAt: '2099-07-11T00:00:00Z', mustRotateBootstrapPassword: false };
  await page.route(`**${api}/auth/admin/session`, (route) => route.fulfill(authenticated ? json(session) : unauthorized()));
  await page.route(`**${api}/auth/admin/login`, (route) => {
    authenticated = true;
    return route.fulfill(json(session));
  });
  await page.route(`**${api}/health/ready`, (route) => route.fulfill(json({ status: 'ready' })));
  await page.route(`**${api}/admin/line-quota`, (route) => route.fulfill(json({
    state: 'READY', providerLimit: 5000, providerConsumed: 4200, locallyAccepted: 24,
    operationalReservePercent: 10, syncedAt: '2026-07-10T12:00:00Z'
  })));
  await page.route(`**${api}/admin/reports`, (route) => route.fulfill(json(adminReportCatalog)));
  await page.route(`**${api}/admin/operational-incidents**`, (route) => route.fulfill(json(isTableQuery(route.request().url()) ? tableResult([]) : { data: [], page: { hasMore: false } })));
}

async function mockEmptyExecutiveOverview(page: Page) {
  await page.route(`**${api}/viewer/tenants/${tenantId}/executive-overview**`, (route) => route.fulfill(json({
    tenantId, timezone: 'Asia/Bangkok', items: []
  })));
}

function salesDashboard() {
  return {
    reportKey: 'sales_goods_services',
    version: '1.0.0',
    period: { preset: 'YESTERDAY', dateFrom: '2026-07-09', dateTo: '2026-07-09' },
    comparisonPeriod: { preset: 'CUSTOM', dateFrom: '2026-07-08', dateTo: '2026-07-08' },
    timezone: 'Asia/Bangkok',
    generatedAt: '2026-07-10T07:00:00+07:00',
    kpis: [{
      key: 'total_amount', label: 'ยอดขาย', value: '1250.00', unit: 'THB',
      comparison: { availability: 'AVAILABLE', previousValue: '1000.00', delta: '250.00', deltaPercent: '25.00', direction: 'UP' }
    }],
    visualizations: [],
    quality: { status: 'OK', warnings: [] }
  };
}

const longThaiLabels = Array.from({ length: 10 }, (_, index) => `สินค้าทดสอบชื่อยาวสำหรับผู้บริหาร อันดับ ${index + 1}`);

function trendVisualization(key: string, title: string) {
  return {
    key, title, intent: 'TREND', unit: 'THB', categories: Array.from({ length: 10 }, (_, index) => String(index + 1)),
    series: [
      { key: 'current', label: 'งวดปัจจุบัน', values: Array.from({ length: 10 }, (_, index) => String((index + 1) * 1000)), pointLabels: Array.from({ length: 10 }, (_, index) => `2026-07-${String(index + 1).padStart(2, '0')}`) },
      { key: 'previous', label: 'งวดก่อน', values: Array.from({ length: 10 }, (_, index) => String((index + 1) * 900)), pointLabels: Array.from({ length: 10 }, (_, index) => `2026-06-${String(index + 1).padStart(2, '0')}`) }
    ]
  };
}

function rankingVisualization(key: string, title: string, negative = false) {
  return {
    key, title, intent: 'RANKING', unit: 'THB', categories: longThaiLabels,
    series: [{ key: 'value', label: title, values: Array.from({ length: 10 }, (_, index) => String((negative ? -1 : 1) * (10_000_000 / (index + 1)))) }]
  };
}

function compositionVisualization(key: string, title: string, multiple = false) {
  const categories = multiple ? longThaiLabels : ['เงินสด', 'เงินโอน', 'บัตร', 'เช็ค'];
  return {
    key, title, intent: 'COMPOSITION', unit: 'THB', categories,
    series: multiple
      ? [{ key: 'in', label: 'รับเข้า', values: categories.map((_, index) => String(1000 - index * 25)) }, { key: 'out', label: 'จ่ายออก', values: categories.map((_, index) => String(700 - index * 20)) }]
      : [{ key: 'amount', label: 'จำนวนเงิน', values: ['25', '75', '40', '10'] }]
  };
}

const visualizationsByReport: Record<string, ReturnType<typeof trendVisualization>[]> = {
  sales_goods_services: [trendVisualization('sales_trend', 'แนวโน้มยอดขาย'), rankingVisualization('top_products', 'สินค้าทำยอดขายสูงสุด')],
  purchase_goods_payables: [trendVisualization('purchase_trend', 'แนวโน้มยอดซื้อ'), rankingVisualization('top_suppliers', 'ผู้จำหน่ายที่มียอดซื้อสูงสุด')],
  gross_profit_by_product: [rankingVisualization('top_profit_products', 'สินค้ากำไรสูงสุด'), rankingVisualization('loss_products', 'สินค้าที่ขาดทุน', true)],
  gross_profit_by_ar_customer: [rankingVisualization('top_profit_customers', 'ลูกค้าที่สร้างกำไรสูงสุด'), rankingVisualization('loss_customers', 'ลูกค้าที่ขาดทุน', true)],
  stock_balance: [rankingVisualization('top_stock_value', 'สินค้าที่มีมูลค่าคงเหลือสูงสุด'), compositionVisualization('stock_movement', 'มูลค่ารับเข้าและจ่ายออกตามสินค้า', true)],
  stock_reorder: [rankingVisualization('reorder_shortage_ratio', 'สินค้าที่ต่ำกว่าจุดสั่งซื้อมากที่สุด')],
  ar_customer_movement: [rankingVisualization('customer_net_movement', 'ลูกหนี้ที่มียอดเคลื่อนไหวสุทธิสูงสุด'), compositionVisualization('customer_debit_credit', 'ยอดเพิ่มและยอดลดตามลูกหนี้', true)],
  ar_debt_receipt: [trendVisualization('debt_receipt_trend', 'แนวโน้มรับชำระหนี้'), compositionVisualization('debt_receipt_methods', 'วิธีรับชำระ')],
  cash_bank_receipts: [trendVisualization('cash_receipt_trend', 'แนวโน้มรับเงิน'), compositionVisualization('cash_receipt_methods', 'ช่องทางการชำระ')],
  cash_bank_payments: [trendVisualization('cash_payment_trend', 'แนวโน้มจ่ายเงิน'), compositionVisualization('cash_payment_methods', 'ช่องทางการชำระ')]
};

if (Object.values(visualizationsByReport).flat().length !== 19) throw new Error('The executive chart fixture must cover all 19 visualizations.');

function dashboardForReport(reportKey: string) {
  return { ...salesDashboard(), reportKey, visualizations: visualizationsByReport[reportKey] ?? [] };
}

test('admin guard redirects an anonymous user to sign in', async ({ page }) => {
  const consoleErrors = captureUnexpectedConsoleErrors(page);
  await page.route(`**${api}/auth/admin/session`, (route) => route.fulfill(unauthorized()));

  await page.goto('/admin/tenants');

  await expect(page).toHaveURL(/\/admin\/login\?redirect=/);
  await expect(page.getByRole('heading', { name: 'Nextstep Admin' })).toBeVisible();
  expect(consoleErrors).toEqual([]);
});

test('admin can sign in and sees API readiness', async ({ page }) => {
  const consoleErrors = captureUnexpectedConsoleErrors(page);
  await mockAdminLogin(page);
  await page.goto('/admin/login');

  await page.getByLabel('รหัสผ่าน').fill('local-e2e-password');
  await page.getByRole('button', { name: 'เข้าสู่ระบบ' }).click();

  await expect(page).toHaveURL('/admin');
  await expect(page.getByRole('heading', { name: 'ภาพรวมระบบ' })).toBeVisible();
  await expect(page.getByText('ระบบพร้อมใช้งาน')).toBeVisible();
  await expect(page.getByText('ใช้แล้ว 4,200 / 5,000')).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'การเข้าถึง' })).toHaveCount(0);
  await expect(page.getByText('ข้อมูลอ่อนไหว')).toHaveCount(0);
  await expect(page.getByText('ข้อมูลทั่วไป')).toHaveCount(0);
  expect(consoleErrors).toEqual([]);
});

test('application uses the shared blue primary tokens in light and dark modes', async ({ page }) => {
  const consoleErrors = captureUnexpectedConsoleErrors(page);
  await mockAdminLogin(page);
  await page.goto('/admin/login');
  await page.getByLabel('รหัสผ่าน').fill('local-e2e-password');
  await page.getByRole('button', { name: 'เข้าสู่ระบบ' }).click();

  await expect(page.getByRole('heading', { name: 'ภาพรวมระบบ' })).toBeVisible();
  expect(await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue('--p-primary-color').trim())).toBe('#3b82f6');
  await expect(page.getByRole('button', { name: 'สลับโหมดสี' })).toBeVisible();

  await page.getByRole('button', { name: 'สลับโหมดสี' }).click();

  await expect(page.locator('html')).toHaveClass(/app-dark/);
  expect(await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue('--p-primary-color').trim())).toBe('#60a5fa');
  expect(consoleErrors).toEqual([]);
});

test('mobile admin topbar shows the current route instead of duplicating the brand', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockAdminLogin(page);
  await page.goto('/admin/login');
  await page.getByLabel('รหัสผ่าน').fill('local-e2e-password');
  await page.getByRole('button', { name: 'เข้าสู่ระบบ' }).click();

  await expect(page.getByTestId('mobile-topbar-context')).toContainText('ภาพรวมระบบ');
  await expect(page.getByTestId('mobile-topbar-context')).toContainText('Nextstep Admin');
  await expect(page.getByText('NEXTSTEP', { exact: true })).toBeHidden();
  for (const width of [320, 390, 768]) {
    await page.setViewportSize({ width, height: width === 768 ? 1024 : 844 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)).toBe(false);
  }
  await page.setViewportSize({ width: 320, height: 844 });
  await page.getByTestId('mobile-topbar-context').locator('strong, span').evaluateAll((elements) => {
    elements.forEach((element) => { (element as HTMLElement).style.fontSize = '200%'; });
  });
  await expect(page.getByTestId('mobile-topbar-context')).toContainText('ภาพรวมระบบ');
  expect(await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)).toBe(false);
});

test('admin operation tables identify the tenant and LINE recipient', async ({ page }) => {
  const session = { username: 'superadmin', expiresAt: '2026-07-11T00:00:00Z', mustRotateBootstrapPassword: false };
  const pageInfo = { hasMore: false };
  await page.route(`**${api}/auth/admin/session`, (route) => route.fulfill(json(session)));
  await page.route(`**${api}/admin/reports`, (route) => route.fulfill(json(adminReportCatalog)));
  const tenantRows = [{ id: tenantId, slug: 'sample-shop', name: 'ร้านตัวอย่าง', timezone: 'Asia/Bangkok', status: 'ACTIVE', accessEndsAt: '2027-07-10T00:00:00Z', version: 1, smlReadiness: 'READY', createdAt: '2026-07-01T00:00:00Z', updatedAt: '2026-07-10T00:00:00Z' }];
  await page.route(`**${api}/admin/tenants**`, (route) => route.fulfill(json(isTableQuery(route.request().url()) ? tableResult(tenantRows) : { data: tenantRows, page: pageInfo })));
  const failedRun = {
    id: '77777777-7777-4777-8777-777777777777', tenantId, tenantName: 'ร้านตัวอย่าง',
    reportKey: 'stock_balance', status: 'FAILED', periodPreset: 'YESTERDAY', dateFrom: '2026-07-10', dateTo: '2026-07-10',
    rowCount: 0, isTruncated: false, queuedAt: '2026-07-10T15:00:00Z', startedAt: '2026-07-10T15:00:01Z',
    finishedAt: '2026-07-10T15:00:04Z', expiresAt: '2026-07-11T15:00:04Z', safeErrorCode: 'SML_UNREACHABLE',
    failureSummary: {
      version: 1, level: 'CONFIRMED', category: 'JAVA_WS_CONNECTIVITY', stage: 'CONNECT_JAVA_WS',
      transportPhase: 'BEFORE_REQUEST_SENT', occurredAt: '2026-07-10T15:00:04Z', durationMs: 3000,
      attempt: 1, retryable: true, remoteStateUnknown: false, connectionVersion: 2, safeErrorCode: 'SML_UNREACHABLE',
      presentation: {
        titleTh: 'ติดต่อ Java Web Service ของร้านไม่สำเร็จ',
        summaryTh: 'ระบบไม่สามารถเริ่มส่งคำขอไปยัง Server ลูกค้าได้',
        stageTh: 'เชื่อมต่อ Java Web Service',
        nextActionsTh: ['ตรวจสอบ Java Web Service', 'ตรวจสอบ Network และ Port']
      }
    }
  };
  await page.route(`**${api}/admin/report-runs**`, (route) => {
    const pathname = new URL(route.request().url()).pathname;
    if (pathname.endsWith(failedRun.id)) {
      return route.fulfill(json({ ...failedRun, triggerKind: 'SCHEDULED', connectionChangedSinceFailure: false, impact: { reportsTotal: 10, reportsSucceeded: 0, reportsFailed: 1, reportsCancelled: 9, notificationOutcome: 'NOT_CREATED_INCOMPLETE_REPORT_SET' } }));
    }
    return route.fulfill(json(isTableQuery(route.request().url()) ? tableResult([failedRun]) : { data: [failedRun], page: pageInfo }));
  });
  const deliveryRows = [{ id: '88888888-8888-4888-8888-888888888888', tenantId, tenantName: 'ร้านตัวอย่าง', recipientDisplayName: 'เจ้าของร้าน', reportKeys: ['sales_goods_services'], reportCount: 1, status: 'ACCEPTED', attempt: 1, acceptedAt: '2026-07-10T15:02:00Z', createdAt: '2026-07-10T15:01:00Z', expiresAt: '2027-07-10T15:01:00Z' }];
  await page.route(`**${api}/admin/line-deliveries**`, (route) => route.fulfill(json(isTableQuery(route.request().url()) ? tableResult(deliveryRows) : { data: deliveryRows, page: pageInfo })));
  const auditRows = [{ id: '99999999-9999-4999-8999-999999999999', tenantId, tenantName: 'ร้านตัวอย่าง', actorType: 'ADMIN', action: 'SCHEDULE_TEST_SEND_ENQUEUED', resourceType: 'SCHEDULE', result: 'SUCCESS', createdAt: '2026-07-10T15:01:00Z' }];
  await page.route(`**${api}/admin/audit-logs**`, (route) => route.fulfill(json(isTableQuery(route.request().url()) ? tableResult(auditRows) : { data: auditRows, page: pageInfo })));

  await page.goto('/admin/report-runs');
  await expect(page.getByRole('columnheader', { name: 'ร้านค้า' })).toBeVisible();
  await expect(page.getByRole('row').filter({ hasText: 'ร้านตัวอย่าง' })).toBeVisible();
  await expect(page.getByRole('row').filter({ hasText: 'ติดต่อ Java Web Service ของร้านไม่สำเร็จ' })).toBeVisible();
  await page.getByLabel('ดูสาเหตุและหลักฐาน').click();
  await expect(page.getByRole('dialog')).toContainText('เชื่อมต่อ Java Web Service');
  await expect(page.getByRole('dialog')).toContainText('ยกเลิก 9');
  await expect(page.getByRole('dialog')).toContainText('ไม่ได้ส่ง LINE');
  await page.keyboard.press('Escape');

  await page.goto('/admin/deliveries');
  await expect(page.getByRole('columnheader', { name: 'ร้านค้า' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'ผู้รับ' })).toBeVisible();
  await expect(page.getByRole('row').filter({ hasText: 'ร้านตัวอย่าง' }).filter({ hasText: 'เจ้าของร้าน' })).toBeVisible();

  await page.goto('/admin/audit');
  await expect(page.getByRole('columnheader', { name: 'ร้านค้า' })).toBeVisible();
  await expect(page.getByRole('row').filter({ hasText: 'ร้านตัวอย่าง' })).toBeVisible();
});

test('admin incident flow separates acknowledgement from evidence-based recovery', async ({ page }) => {
  const consoleErrors = captureUnexpectedConsoleErrors(page);
  await mockAdminLogin(page);
  await page.setViewportSize({ width: 1440, height: 1000 });
  const incidentId = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';
  const incidentTenantId = 'cccccccc-cccc-4ccc-8ccc-cccccccccccc';
  const occurrenceId = 'dddddddd-dddd-4ddd-8ddd-dddddddddddd';
  let status = 'OPEN';
  let connectionTestCount = 0;
  let diagnosisCount = 0;
  const incident = () => ({
    id: incidentId, alertRef: 'NST-ABC123DEF456', incidentType: 'SCHEDULED_REPORT_FAILED',
    rootCause: 'SML_CONNECTIVITY', severity: 'P1', status, safeErrorCode: 'SML_UNREACHABLE',
    occurrenceCount: 2, affectedCount: 3, activeAffectedCount: 3, observationMode: 'DISCRETE', subjectType: 'TENANT',
    tenantExamples: ['ร้านตัวอย่างหนึ่ง', 'ร้านตัวอย่างสอง'], firstSeenAt: '2026-07-16T01:00:00Z',
    lastSeenAt: '2026-07-16T01:01:00Z', version: status === 'OPEN' ? 1 : 2,
    ...(status === 'RESOLVED' ? { resolvedAt: '2026-07-16T01:06:00Z' } : {}),
    presentation: { titleTh: 'ติดต่อ Java Web Service ของร้านไม่สำเร็จ', summaryTh: 'ระบบไม่สามารถเริ่มส่งคำขอไปยัง Server ลูกค้าได้', stageTh: 'เชื่อมต่อ Java Web Service', nextActionsTh: ['ตรวจสอบ Java Web Service'] },
    statusPresentation: status === 'RESOLVED'
      ? { state: 'CONNECTION_RESTORED', headlineTh: 'เชื่อมต่อ Java Web Service ได้แล้ว', statusSummaryTh: 'ไม่ต้องดำเนินการ', verifiedAt: '2026-07-16T01:06:00Z', actionRequired: false }
      : { state: 'CONNECTION_FAILED', headlineTh: 'เชื่อมต่อ Java Web Service ไม่สำเร็จ', statusSummaryTh: 'ควรตรวจสอบ Java Web Service, Network และ Server ของลูกค้า', actionRequired: true },
    causeBreakdown: [{
      presentation: { titleTh: 'ติดต่อ Java Web Service ของร้านไม่สำเร็จ', summaryTh: 'ระบบไม่สามารถเริ่มส่งคำขอไปยัง Server ลูกค้าได้', stageTh: 'เชื่อมต่อ Java Web Service', nextActionsTh: ['ตรวจสอบ Java Web Service'] },
      category: 'JAVA_WS_CONNECTIVITY', stage: 'CONNECT_JAVA_WS', transportPhase: 'BEFORE_REQUEST_SENT', investigationScope: 'CUSTOMER_SYSTEM', subjectType: 'TENANT',
      occurrenceCount: 2, affectedCount: 3, activeAffectedCount: 3, affectedLabelTh: 'ร้านที่ได้รับผล', firstSeenAt: '2026-07-16T01:00:00Z', lastSeenAt: '2026-07-16T01:01:00Z'
    }],
    isDownstream: false
  });
  await page.route(`**${api}/admin/operational-incidents**`, async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith('/acknowledge')) {
      status = 'ACKNOWLEDGED';
      await route.fulfill(json(incident()));
      return;
    }
    if (url.pathname.endsWith(`/occurrences/${occurrenceId}/diagnosis`)) {
      diagnosisCount++;
      await route.fulfill(json({
        assessment: {
          problemArea: 'CUSTOMER_JAVA_WS', investigationOwner: 'CUSTOMER_IT', loadSignal: 'NO_NEXTSTEP_LOAD_SIGNAL',
          summaryTh: 'เชื่อมต่อ Java Web Service สำเร็จ แต่ข้อมูลตอบกลับไม่ใช่ ZIP',
          problemAreaTh: 'คำตอบจาก Java Web Service', ownerTh: 'ผู้ดูแล Java Web Service ของลูกค้า',
          loadSignalTh: 'ไม่พบสัญญาณว่า Nextstep สร้างภาระผิดปกติ',
          customerActionTh: 'ตรวจสอบ Java Web Service log ด้วย Request Ref โดยไม่ต้อง Restart Server จากหลักฐานนี้'
        },
        protocolEvidence: {
          requestRef: 'NXR-E2E4C9F7D120', requestCount: 1, retryCount: 0,
          requestSentAt: '2026-07-16T01:00:00Z', firstResponseByteAt: '2026-07-16T01:00:00.032Z',
          responseCompletedAt: '2026-07-16T01:00:00.064Z', httpStatus: 200,
          responseContentType: 'application/soap+xml', responseBodyBytes: 384,
          soapValid: true, soapReturnCharacters: 128, base64Valid: true,
          decodedPayloadBytes: 64, zipSignatureValid: false,
          tenantConcurrentQueries: 1, hostConcurrentQueries: 1
        },
        priorMatchingSuccess: { finishedAt: '2026-07-15T01:00:00Z', durationMs: 61 },
        subsequentMatchingSuccess: { finishedAt: '2026-07-17T01:00:00Z', durationMs: 66 },
        baseline: { p50Ms: 62, p90Ms: 80, sampleCount: 8 },
        customerMessageTh: 'ร้าน: ร้านตัวอย่างหนึ่ง\nเวลา: 16 ก.ค. 2569 08:00 น.\nRequest Ref: NXR-E2E4C9F7D120\nส่งคำขอ 1 ครั้ง ไม่มี Retry และได้รับ HTTP 200 แต่ข้อมูลไม่ใช่ ZIP'
      }));
      return;
    }
    if (url.pathname.endsWith('/occurrences') || url.pathname.endsWith('/occurrences/query')) {
      const occurrences = [{
        id: occurrenceId, tenantId: incidentTenantId, tenantName: 'ร้านตัวอย่างหนึ่ง', reportKey: 'stock_balance',
        sourceKind: 'REPORT', safeErrorCode: 'SML_ZIP_FORMAT_INVALID', observedAt: '2026-07-16T01:00:00Z',
        failureEvidence: {
          version: 2, level: 'CONFIRMED', category: 'JAVA_WS_RESPONSE', stage: 'DECODE_PAYLOAD', transportPhase: 'RESPONSE_STARTED',
          occurredAt: '2026-07-16T01:00:00Z', durationMs: 64, retryable: false, remoteStateUnknown: false,
          safeErrorCode: 'SML_ZIP_FORMAT_INVALID', presentation: incident().presentation
        },
        impact: { reportsTotal: 10, reportsSucceeded: 9, reportsFailed: 1, reportsCancelled: 0, notificationOutcome: 'NOT_CREATED_INCOMPLETE_REPORT_SET' },
        smlConnectionReference: {
          endpointUrlAtFailure: 'http://sml.example.test:8092', currentEndpointUrl: 'http://sml.example.test:8092', endpointHost: 'sml.example.test:8092',
          versionAtFailure: 2, currentVersion: 2, status: 'EXACT_VERSION', schemeSecurity: 'HTTP'
        }
      }];
      await route.fulfill(json(url.pathname.endsWith('/query') ? tableResult(occurrences) : { data: occurrences, page: { hasMore: false } }));
      return;
    }
    if (url.pathname.endsWith(incidentId)) {
      await route.fulfill(json({ ...incident(), events: [{
        id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', eventKind: 'OBSERVED', sourceKind: 'REPORT',
        safeErrorCode: 'SML_UNREACHABLE', tenantName: 'ร้านตัวอย่างหนึ่ง', reportKey: 'stock_balance', observedAt: '2026-07-16T01:00:00Z',
        isDownstream: false, connectionChangedSinceFailure: false,
        failureEvidence: {
          version: 1, level: 'CONFIRMED', category: 'JAVA_WS_CONNECTIVITY', stage: 'CONNECT_JAVA_WS', transportPhase: 'BEFORE_REQUEST_SENT',
          occurredAt: '2026-07-16T01:00:00Z', durationMs: 3000, retryable: true, remoteStateUnknown: false,
          safeErrorCode: 'SML_UNREACHABLE', presentation: incident().presentation
        },
        impact: { reportsTotal: 1, reportsSucceeded: 0, reportsFailed: 1, reportsCancelled: 0, notificationOutcome: 'NOT_APPLICABLE' }
      }] }));
      return;
    }
    await route.fulfill(json(url.pathname.endsWith('/query') ? tableResult([incident()]) : { data: [incident()], page: { hasMore: false } }));
  });
  await page.route(`**${api}/admin/tenants/${incidentTenantId}/sml-connection/test`, async (route) => {
    connectionTestCount++;
    await route.fulfill(json({ status: 'READY', testedAt: '2026-07-16T01:05:00Z', latencyMs: 320 }));
  });

  await page.goto('/admin/login');
  await page.getByLabel('รหัสผ่าน').fill('local-e2e-password');
  await page.getByRole('button', { name: 'เข้าสู่ระบบ' }).click();
  await page.goto('/admin/operational-incidents');
  await expect(page.getByRole('heading', { name: 'เหตุสำคัญ' })).toBeVisible();
  await expect(page.getByText('NST-ABC123DEF456')).toBeVisible();
  await expect(page.getByText('ร้านตัวอย่างหนึ่ง · ร้านตัวอย่างสอง และอีก 1 ร้าน')).toBeVisible();
  const incidentRow = page.getByRole('row').filter({ hasText: 'NST-ABC123DEF456' });
  const openIncident = incidentRow.getByLabel('เปิดรายละเอียดเหตุสำคัญ');
  await expect(openIncident).toBeVisible();
  await Promise.all([
    page.waitForURL(`/admin/operational-incidents/${incidentId}`),
    openIncident.click()
  ]);
  await expect(page.getByRole('heading', { name: 'รายละเอียดเหตุสำคัญ' })).toBeVisible();
  await expect(page.getByText('เชื่อมต่อ Java Web Service ไม่สำเร็จ', { exact: true }).first()).toBeVisible();
  await expect(page.getByRole('heading', { name: 'ร้านตัวอย่างหนึ่ง' })).toBeVisible();
  await expect(page.getByText('เชื่อมต่อ Java Web Service สำเร็จ แต่ข้อมูลตอบกลับไม่ใช่ ZIP')).toBeVisible();
  await expect(page.getByText('คำตอบจาก Java Web Service', { exact: true })).toBeVisible();
  await expect(page.getByText('ผู้ดูแล Java Web Service ของลูกค้า', { exact: true }).first()).toBeVisible();
  await expect(page.getByText('ไม่พบสัญญาณว่า Nextstep สร้างภาระผิดปกติ', { exact: true })).toBeVisible();
  await expect(page.getByText('1 ครั้ง · ไม่มี Retry', { exact: true })).toBeVisible();
  await expect(page.getByText('พบรายงานเงื่อนไขเดียวกันที่สำเร็จทั้งก่อนและหลังเหตุ')).toBeVisible();
  await expect(page.getByRole('button', { name: 'คัดลอกคำตอบให้ลูกค้า' })).toBeVisible();
  expect(diagnosisCount).toBe(1);
  await expect(page.getByRole('heading', { name: 'ลำดับสาเหตุและผลกระทบ' })).toHaveCount(0);
  await page.getByRole('button', { name: 'หลักฐานและรายละเอียดเพิ่มเติม' }).click();
  await expect(page.getByRole('heading', { name: 'ลำดับสาเหตุและผลกระทบ' })).toBeVisible();
  await expect(page.getByText('SML_UNREACHABLE', { exact: true })).toBeVisible();
  await expect(page.getByText('การเชื่อมต่อนี้ไม่ได้เข้ารหัส')).toBeVisible();
  const openEndpoint = page.getByRole('link', { name: 'เปิด URL ใน Browser' });
  await expect(openEndpoint).toHaveAttribute('href', 'http://sml.example.test:8092');
  await expect(openEndpoint).toHaveAttribute('rel', 'noopener noreferrer');
  expect(connectionTestCount).toBe(0);
  await page.getByRole('button', { name: 'ทดสอบจาก Server Dashboard' }).click();
  await page.getByRole('button', { name: 'เริ่มทดสอบ' }).click();
  await expect(page.getByText(/สำเร็จเมื่อ/)).toBeVisible();
  expect(connectionTestCount).toBe(1);
  await expect(page.getByRole('button', { name: 'รับทราบ' })).toBeVisible();
  await expect(page.getByRole('button', { name: /หายแล้ว|Resolve/i })).toHaveCount(0);
  await page.getByRole('button', { name: 'รับทราบ' }).click();
  await expect(page.getByText('รับทราบแล้ว')).toBeVisible();
  await expect(page.getByText('ระบบยืนยันว่าหายแล้ว')).toHaveCount(0);
  status = 'RESOLVED';
  await page.reload();
  await expect(page.getByText('เชื่อมต่อ Java Web Service ได้แล้ว', { exact: true }).first()).toBeVisible();
  await expect(page.getByText('ไม่ต้องดำเนินการ', { exact: true }).first()).toBeVisible();
  await expect(page.getByText('ระบบยืนยันว่าหายแล้ว')).toHaveCount(0);
  await page.setViewportSize({ width: 768, height: 1024 });
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)).toBe(false);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)).toBe(false);
  expect(consoleErrors).toEqual([]);
});

test('viewer loading messages are centered', async ({ page }) => {
  let releaseViewer: (() => void) | undefined;
  const viewerRequested = new Promise<void>((resolve) => { releaseViewer = resolve; });
  await page.route(`**${api}/viewer/me`, async (route) => {
    await viewerRequested;
    await route.fulfill(unauthorized());
  });

  await page.goto('/app');
  const loadingHeading = page.getByRole('heading', { name: 'กำลังยืนยัน LINE' });
  await expect(loadingHeading).toBeVisible();
  await expect(loadingHeading).toHaveCSS('text-align', 'center');
  releaseViewer?.();
});

test('admin login ignores an external redirect target', async ({ page }) => {
  await mockAdminLogin(page);
  await page.goto('/admin/login?redirect=https://example.com/phishing');
  await page.getByLabel('รหัสผ่าน').fill('local-e2e-password');
  await page.getByRole('button', { name: 'เข้าสู่ระบบ' }).click();
  await expect(page).toHaveURL('/admin');
});

test('tenant form creates an internal slug and uses the admin-selected access end date', async ({ page }) => {
  await mockAdminLogin(page);
  let createRequests = 0;
  await page.route(`**${api}/admin/tenants**`, async (route) => {
    if (isTableQuery(route.request().url())) {
      await route.fulfill(json(tableResult([])));
      return;
    }
    if (route.request().method() === 'POST') {
      createRequests++;
      expect(route.request().postDataJSON()).toEqual({ name: 'ร้านทดสอบ', accessEndsAt: '2027-12-31T16:59:59.999Z' });
      await route.fulfill(json({ id: tenantId, slug: 'shop-k7m2p9x4abcd', name: 'ร้านทดสอบ', timezone: 'Asia/Bangkok', status: 'DISABLED', accessEndsAt: '2027-07-10T00:00:00Z', version: 1, smlReadiness: 'UNCONFIGURED', createdAt: '2026-07-10T00:00:00Z', updatedAt: '2026-07-10T00:00:00Z' }, 201));
      return;
    }
    await route.fulfill(json({ data: [], page: { hasMore: false } }));
  });
  await page.goto('/admin/login');
  await page.getByLabel('รหัสผ่าน').fill('local-e2e-password');
  await page.getByRole('button', { name: 'เข้าสู่ระบบ' }).click();
  await page.goto('/admin/tenants');

  await page.getByRole('button', { name: 'เพิ่มร้านค้า' }).click();
  await page.getByLabel('ชื่อร้าน').fill('ร้านทดสอบ');
  await page.getByLabel('สิ้นสุดสิทธิ์').fill('31/12/2027');
  await page.getByLabel('สิ้นสุดสิทธิ์').press('Tab');
  await expect(page.getByLabel('Slug', { exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: 'สร้างร้านค้า' }).click();
  await expect.poll(() => createRequests).toBe(1);
});

test('server-side column filter waits for Apply before querying', async ({ page }) => {
  await mockAdminLogin(page);
  const queryBodies: unknown[] = [];
  await page.route(`**${api}/admin/tenants/query`, async (route) => {
    queryBodies.push(route.request().postDataJSON());
    await route.fulfill(json(tableResult([])));
  });
  await page.goto('/admin/login');
  await page.getByLabel('รหัสผ่าน').fill('local-e2e-password');
  await page.getByRole('button', { name: 'เข้าสู่ระบบ' }).click();
  await page.goto('/admin/tenants');
  await expect.poll(() => queryBodies.length).toBe(1);

  const statusHeader = page.getByRole('columnheader', { name: /สถานะ/ });
  await statusHeader.getByRole('button', { name: 'แสดงเมนูตัวกรอง' }).click();
  await page.getByRole('dialog').locator('.p-multiselect').click();
  await page.getByRole('option', { name: 'ใช้งาน', exact: true }).click();
  await page.keyboard.press('Escape');
  await expect.poll(() => queryBodies.length).toBe(1);

  await statusHeader.getByRole('button', { name: 'ซ่อนเมนูตัวกรอง' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await statusHeader.getByRole('button', { name: 'แสดงเมนูตัวกรอง' }).click();
  await page.getByRole('button', { name: 'ใช้ตัวกรอง' }).click();
  await expect.poll(() => queryBodies.length).toBe(2);
  expect(queryBodies[1]).toMatchObject({ page: 0, pageSize: 25, filters: { statuses: [] } });

  await statusHeader.getByRole('button', { name: 'แสดงเมนูตัวกรอง' }).click();
  await page.getByRole('dialog').locator('.p-multiselect').click();
  await page.getByRole('option', { name: 'ใช้งาน', exact: true }).click();
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'ใช้ตัวกรอง' }).click();
  await expect.poll(() => queryBodies.length).toBe(3);
  expect(queryBodies[2]).toMatchObject({ page: 0, pageSize: 25, filters: { statuses: ['ACTIVE'] } });
});

test('tenant detail restores the editable SML URL, removes duplicate entity header, and keeps the tenant menu active', async ({ page }) => {
  const session = { username: 'superadmin', expiresAt: '2026-07-11T00:00:00Z', mustRotateBootstrapPassword: false };
  await page.route(`**${api}/auth/admin/session`, (route) => route.fulfill(json(session)));
  await page.route(`**${api}/admin/tenants/${tenantId}`, (route) => route.fulfill(json({
    id: tenantId, slug: 'sample-shop', name: 'วาวา', timezone: 'Asia/Bangkok', status: 'ACTIVE',
    accessEndsAt: '2027-07-10T00:00:00Z', version: 1, smlReadiness: 'READY',
    createdAt: '2026-07-01T00:00:00Z', updatedAt: '2026-07-10T00:00:00Z'
  })));
  await page.route(`**${api}/admin/tenants/${tenantId}/sml-connection`, (route) => route.fulfill(json({
    isConfigured: true, endpointUrl: 'http://43.229.149.11:8080', endpointHost: '43.229.149.11:8080',
    databaseName: 'WAWA2', configFileName: 'SMLConfigDATA.xml', readinessStatus: 'READY', version: 2
  })));
  await page.route(`**${api}/admin/tenants/${tenantId}/recipients**`, (route) => route.fulfill(json(isTableQuery(route.request().url()) ? tableResult([]) : { data: [], page: { hasMore: false } })));
  await page.route(`**${api}/admin/tenants/${tenantId}/schedules**`, (route) => route.fulfill(json(isTableQuery(route.request().url()) ? tableResult([]) : { data: [], page: { hasMore: false } })));

  await page.goto(`/admin/tenants/${tenantId}?tab=sml`);

  await expect(page.locator('.page-header')).toHaveCount(0);
  await expect(page.getByLabel('Java Web Service Base URL')).toHaveValue('http://43.229.149.11:8080');
  await expect(page.getByText('ปลายทางที่ระบบจะทดสอบ:')).toContainText('ปลายทางที่ระบบจะทดสอบ:');
  await expect(page.getByText('http://43.229.149.11:8080/SMLJavaWebService/DotNetFrameWork')).toBeVisible();
  await expect(page.getByText('การเชื่อมต่อ SML พร้อมใช้งาน')).toBeVisible();
  await expect(page.getByText(/การทดสอบส่ง/)).toContainText('select 1');
  await expect(page.getByRole('link', { name: 'ร้านค้า', exact: true })).toHaveClass(/active-route/);
  await expect(page.getByText('เวลาไทย', { exact: true })).toHaveCount(0);
});

test('SML connection cooldown keeps the test safe and tells the admin when to retry', async ({ page }) => {
  const session = { username: 'superadmin', expiresAt: '2026-07-11T00:00:00Z', mustRotateBootstrapPassword: false };
  await page.route(`**${api}/auth/admin/session`, (route) => route.fulfill(json(session)));
  await page.route(`**${api}/admin/tenants/${tenantId}`, (route) => route.fulfill(json({
    id: tenantId, slug: 'sample-shop', name: 'ร้านตัวอย่าง', timezone: 'Asia/Bangkok', status: 'ACTIVE',
    accessEndsAt: '2027-07-10T00:00:00Z', version: 1, smlReadiness: 'READY',
    createdAt: '2026-07-01T00:00:00Z', updatedAt: '2026-07-10T00:00:00Z'
  })));
  await page.route(`**${api}/admin/tenants/${tenantId}/sml-connection/test`, (route) => route.fulfill({
    ...json({ error: { code: 'SML_TEST_COOLDOWN', message: 'SML connection test failed safely.', requestId: 'e2e', retryable: true } }, 429),
    headers: { 'Retry-After': '60' }
  }));
  await page.route(`**${api}/admin/tenants/${tenantId}/sml-connection`, (route) => route.fulfill(json({
    isConfigured: true, endpointUrl: 'https://shop.example.com', endpointHost: 'shop.example.com',
    databaseName: 'DATA1', configFileName: 'SMLConfigDATA.xml', readinessStatus: 'READY', version: 2
  })));
  await page.route(`**${api}/admin/tenants/${tenantId}/recipients**`, (route) => route.fulfill(json(isTableQuery(route.request().url()) ? tableResult([]) : { data: [], page: { hasMore: false } })));
  await page.route(`**${api}/admin/tenants/${tenantId}/schedules**`, (route) => route.fulfill(json(isTableQuery(route.request().url()) ? tableResult([]) : { data: [], page: { hasMore: false } })));

  await page.goto(`/admin/tenants/${tenantId}?tab=sml`);
  await page.getByRole('button', { name: 'ทดสอบการเชื่อมต่อ' }).click();

  await expect(page.getByText('กำลังพักการทดสอบชั่วคราว')).toBeVisible();
  await expect(page.getByText(/ระบบจะไม่เริ่มคำขอซ้ำ/)).toBeVisible();
  await expect(page.getByRole('button', { name: 'ทดสอบการเชื่อมต่อ' })).toBeDisabled();
});

test('an unconfigured tenant starts with the standard SMLConfigDATA filename', async ({ page }) => {
  const session = { username: 'superadmin', expiresAt: '2026-07-11T00:00:00Z', mustRotateBootstrapPassword: false };
  await page.route(`**${api}/auth/admin/session`, (route) => route.fulfill(json(session)));
  await page.route(`**${api}/admin/tenants/${tenantId}`, (route) => route.fulfill(json({
    id: tenantId, slug: 'new-shop', name: 'ร้านใหม่', timezone: 'Asia/Bangkok', status: 'DISABLED',
    accessEndsAt: '2027-07-10T00:00:00Z', version: 1, smlReadiness: 'UNCONFIGURED',
    createdAt: '2026-07-01T00:00:00Z', updatedAt: '2026-07-10T00:00:00Z'
  })));
  await page.route(`**${api}/admin/tenants/${tenantId}/sml-connection`, (route) => route.fulfill(json({
    error: { code: 'SML_CONNECTION_NOT_FOUND', message: 'SML connection is not configured.', requestId: 'e2e', retryable: false }
  }, 404)));
  await page.route(`**${api}/admin/tenants/${tenantId}/recipients**`, (route) => route.fulfill(json(isTableQuery(route.request().url()) ? tableResult([]) : { data: [], page: { hasMore: false } })));
  await page.route(`**${api}/admin/tenants/${tenantId}/schedules**`, (route) => route.fulfill(json(isTableQuery(route.request().url()) ? tableResult([]) : { data: [], page: { hasMore: false } })));

  await page.goto(`/admin/tenants/${tenantId}?tab=sml`);

  await expect(page.getByLabel('ไฟล์ SMLConfig')).toHaveValue('SMLConfigDATA.xml');
});

test('admin manages recipient permissions on a full searchable page with optimistic versioning', async ({ page }) => {
  const recipientId = '33333333-3333-4333-8333-333333333333';
  const session = { username: 'superadmin', expiresAt: '2026-07-11T00:00:00Z', mustRotateBootstrapPassword: false };
  let permissionBody: unknown;
  await page.route(`**${api}/auth/admin/session`, (route) => route.fulfill(json(session)));
  await page.route(`**${api}/admin/reports`, (route) => route.fulfill(json(adminReportCatalog)));
  await page.route(`**${api}/admin/tenants/${tenantId}`, (route) => route.fulfill(json({ id: tenantId, slug: 'shop-sample', name: 'ร้านตัวอย่าง', timezone: 'Asia/Bangkok', status: 'ACTIVE', accessEndsAt: '2027-07-10T00:00:00Z', version: 1, smlReadiness: 'READY', createdAt: '2026-07-01T00:00:00Z', updatedAt: '2026-07-10T00:00:00Z' })));
  await page.route(`**${api}/admin/tenants/${tenantId}/recipients/${recipientId}`, (route) => route.fulfill(json({ id: recipientId, status: 'ACTIVE', displayName: 'เจ้าของร้าน', reportKeys: ['sales_goods_services'], permissionsVersion: 3, verifiedAt: '2026-07-10T00:00:00Z', createdAt: '2026-07-01T00:00:00Z' })));
  await page.route(`**${api}/admin/tenants/${tenantId}/recipients/${recipientId}/permissions`, async (route) => {
    permissionBody = route.request().postDataJSON();
    await route.fulfill(json({ id: recipientId, status: 'ACTIVE', displayName: 'เจ้าของร้าน', reportKeys: ['sales_goods_services', 'stock_balance'], permissionsVersion: 4, verifiedAt: '2026-07-10T00:00:00Z', createdAt: '2026-07-01T00:00:00Z' }));
  });

  await page.goto(`/admin/tenants/${tenantId}/recipients/${recipientId}/permissions`);
  await expect(page.getByRole('link', { name: 'ร้านค้า', exact: true })).toHaveClass(/active-route/);
  await expect(page.getByRole('heading', { name: 'กำหนดสิทธิ์รายงาน' })).toBeVisible();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await page.getByLabel('ค้นหารายงาน').fill('สต็อกคงเหลือ');
  await page.getByLabel('เลือก รายงานสต็อกคงเหลือ').check();
  await expect(page.getByText('เพิ่ม 1 · นำออก 0')).toBeVisible();
  await page.getByRole('button', { name: 'บันทึกสิทธิ์' }).click();
  await expect(page.getByText('บันทึกสิทธิ์แล้ว')).toBeVisible();
  expect(permissionBody).toEqual({ reportKeys: ['sales_goods_services', 'stock_balance'], version: 3 });
});

for (const reportCount of [150, 500]) {
  test(`report picker keeps filtered selections with a ${reportCount}-report catalog`, async ({ page }) => {
    const recipientId = '33333333-3333-4333-8333-333333333333';
    const session = { username: 'superadmin', expiresAt: '2026-07-11T00:00:00Z', mustRotateBootstrapPassword: false };
    const definitions = Array.from({ length: reportCount }, (_, index) => ({
      reportKey: `catalog_report_${String(index).padStart(4, '0')}`,
      label: `รายงานทดสอบ ${String(index).padStart(4, '0')}`,
      category: `CATEGORY_${index % 10}`,
      categoryLabel: `หมวด ${index % 10}`,
      version: '1.0.0',
      status: 'ACTIVE'
    }));
    await page.route(`**${api}/auth/admin/session`, (route) => route.fulfill(json(session)));
    await page.route(`**${api}/admin/reports`, (route) => route.fulfill(json({ data: definitions, limits: { maxScheduleReports: 10, maxFlexPayloadBytes: 30720 } })));
    await page.route(`**${api}/admin/tenants/${tenantId}`, (route) => route.fulfill(json({ id: tenantId, slug: 'shop-sample', name: 'ร้านตัวอย่าง', timezone: 'Asia/Bangkok', status: 'ACTIVE', accessEndsAt: '2027-07-10T00:00:00Z', version: 1, smlReadiness: 'READY', createdAt: '2026-07-01T00:00:00Z', updatedAt: '2026-07-10T00:00:00Z' })));
    await page.route(`**${api}/admin/tenants/${tenantId}/recipients/${recipientId}`, (route) => route.fulfill(json({ id: recipientId, status: 'ACTIVE', displayName: 'เจ้าของร้าน', reportKeys: [], permissionsVersion: 1, verifiedAt: '2026-07-10T00:00:00Z', createdAt: '2026-07-01T00:00:00Z' })));

    await page.goto(`/admin/tenants/${tenantId}/recipients/${recipientId}/permissions`);
    await page.getByLabel('ค้นหารายงาน').fill('000');
    await page.getByRole('button', { name: 'เลือกผลที่กรอง' }).click();
    await expect(page.getByText(`เลือกแล้ว 10 จาก ${reportCount} รายงาน`)).toBeVisible();
    await page.getByLabel('ค้นหารายงาน').fill('0010');
    await page.getByLabel('เลือก รายงานทดสอบ 0010').check();
    await expect(page.getByText(`เลือกแล้ว 11 จาก ${reportCount} รายงาน`)).toBeVisible();
    await page.getByLabel('ค้นหารายงาน').fill('');
    await page.getByLabel('เฉพาะที่เลือก').check();
    await expect(page.getByText('รายงานทดสอบ 0000')).toBeVisible();
    await expect(page.getByText('รายงานทดสอบ 0010')).toBeVisible();
  });
}

test('mobile viewer renders only reports returned by permission API', async ({ page }) => {
  const consoleErrors = captureUnexpectedConsoleErrors(page);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.route(`**${api}/viewer/me`, (route) => route.fulfill(json({
    recipientId: '22222222-2222-4222-8222-222222222222', displayName: 'ผู้ทดสอบ', expiresAt: '2026-07-11T00:00:00Z'
  })));
  await page.route(`**${api}/viewer/tenants`, (route) => route.fulfill(json({
    data: [{ id: tenantId, name: 'ร้านตัวอย่าง', timezone: 'Asia/Bangkok', reportKeys: ['sales_goods_services', 'stock_reorder'] }],
    page: { hasMore: false }
  })));
  await page.route(`**${api}/viewer/tenants/${tenantId}/reports`, (route) => route.fulfill(json({
    data: [
      { reportKey: 'sales_goods_services', version: '1.0.0', label: 'รายงานขายสินค้าและบริการ', category: 'SALES', isSensitive: false },
      { reportKey: 'stock_reorder', version: '1.0.0', label: 'รายงานสินค้าถึงจุดสั่งซื้อ', category: 'INVENTORY', isSensitive: false }
    ],
    page: { hasMore: false }
  })));
  await mockEmptyExecutiveOverview(page);

  await page.goto('/app');

  await expect(page).toHaveURL(`/app/tenant/${tenantId}`);
  await expect(page.getByTestId('mobile-topbar-context')).toContainText('ร้านตัวอย่าง');
  await expect(page.getByTestId('mobile-topbar-context')).toContainText('ภาพรวม');
  await expect(page.getByRole('heading', { name: 'ยังไม่มี Snapshot สำหรับช่วงนี้' })).toBeVisible();
  const hiddenPageHeading = await page.getByRole('heading', { name: 'ภาพรวมร้าน ร้านตัวอย่าง' }).boundingBox();
  expect(hiddenPageHeading?.width).toBeLessThanOrEqual(1);
  await page.getByRole('button', { name: 'เปิดหรือปิดเมนู' }).click();
  await expect(page.getByRole('link', { name: 'รายงานขายสินค้าและบริการ' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'รายงานสินค้าถึงจุดสั่งซื้อ' })).toBeVisible();
  await expect(page.getByText('รายงานซื้อสินค้าและตั้งหนี้')).toHaveCount(0);
  const hasHorizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
  expect(hasHorizontalOverflow).toBe(false);
  for (const label of ['เปิดหรือปิดเมนู', 'สลับโหมดสี', 'ออกจากระบบ']) {
    const box = await page.getByRole('button', { name: label }).boundingBox();
    expect(box?.width).toBeGreaterThanOrEqual(44);
    expect(box?.height).toBeGreaterThanOrEqual(44);
  }
  await page.getByRole('button', { name: 'เปิดหรือปิดเมนู' }).press('Escape');
  await expect.poll(() => page.evaluate(() => document.body.classList.contains('blocked-scroll'))).toBe(false);
  expect(consoleErrors).toEqual([]);
});

test('mobile viewer switches tenants from the sidebar without showing stale topbar context', async ({ page }) => {
  const secondTenantId = '22222222-2222-4222-8222-222222222223';
  await page.setViewportSize({ width: 390, height: 844 });
  await page.route(`**${api}/viewer/me`, (route) => route.fulfill(json({ recipientId: '22222222-2222-4222-8222-222222222222', displayName: 'ผู้ทดสอบ', expiresAt: '2026-07-12T00:00:00Z' })));
  await page.route(`**${api}/viewer/tenants`, (route) => route.fulfill(json({
    data: [
      { id: tenantId, name: 'ร้านหนึ่ง', timezone: 'Asia/Bangkok', reportKeys: ['sales_goods_services'] },
      { id: secondTenantId, name: 'ร้านสอง', timezone: 'Asia/Bangkok', reportKeys: ['sales_goods_services'] }
    ],
    page: { hasMore: false }
  })));
  await page.route(`**${api}/viewer/tenants/*/reports`, async (route) => {
    if (route.request().url().includes(secondTenantId)) await new Promise((resolve) => setTimeout(resolve, 250));
    await route.fulfill(json({ data: [{ reportKey: 'sales_goods_services', version: '1.0.0', label: 'รายงานขายสินค้าและบริการ', category: 'SALES', isSensitive: false }], page: { hasMore: false } }));
  });
  await page.route(`**${api}/viewer/tenants/*/executive-overview`, (route) => route.fulfill(json({
    tenantId: route.request().url().includes(secondTenantId) ? secondTenantId : tenantId, timezone: 'Asia/Bangkok', items: []
  })));

  await page.goto(`/app/tenant/${tenantId}`);
  await expect(page.getByTestId('mobile-topbar-context')).toContainText('ร้านหนึ่ง');
  await page.getByRole('button', { name: 'เปิดหรือปิดเมนู' }).click();
  await page.getByRole('combobox', { name: 'ร้านที่กำลังดู' }).click();
  await page.getByRole('option', { name: 'ร้านสอง' }).click();

  await expect(page).toHaveURL(`/app/tenant/${secondTenantId}`);
  await expect(page.getByTestId('mobile-topbar-context')).toContainText('ร้านสอง');
  await expect(page.getByTestId('mobile-topbar-context')).not.toContainText('ร้านหนึ่ง');
});

test('viewer refreshes SQL only after confirming the current tenant context', async ({ page }) => {
  let refreshRequests = 0;
  let automaticRevalidationRequests = 0;
  let exactCacheLookupRequests = 0;
  const refreshId = '44444444-4444-4444-8444-444444444444';
  const reports = [
    { reportKey: 'sales_goods_services', version: '1.0.0', label: 'รายงานขายสินค้าและบริการ', category: 'SALES', isSensitive: false },
    { reportKey: 'stock_balance', version: '1.0.0', label: 'รายงานสต็อกคงเหลือ', category: 'INVENTORY', isSensitive: false }
  ];
  await page.setViewportSize({ width: 390, height: 844 });
  await page.route(`**${api}/viewer/me`, (route) => route.fulfill(json({ recipientId: '22222222-2222-4222-8222-222222222222', displayName: 'ผู้ทดสอบ', expiresAt: '2026-07-12T00:00:00Z' })));
  await page.route(`**${api}/viewer/tenants`, (route) => route.fulfill(json({ data: [{ id: tenantId, name: 'วาวา', timezone: 'Asia/Bangkok', reportKeys: reports.map((report) => report.reportKey) }], page: { hasMore: false } })));
  await page.route(`**${api}/viewer/tenants/${tenantId}/reports`, (route) => route.fulfill(json({ data: reports, page: { hasMore: false } })));
  await mockEmptyExecutiveOverview(page);
  await page.route(`**${api}/viewer/tenants/${tenantId}/executive-overview**`, (route) => {
    if (new URL(route.request().url()).searchParams.has('periodPreset')) exactCacheLookupRequests++;
    return route.fulfill(json({ tenantId, timezone: 'Asia/Bangkok', items: [] }));
  });
  await page.route(`**${api}/viewer/tenants/${tenantId}/executive-overview/revalidations`, (route) => {
    automaticRevalidationRequests++;
    return route.fulfill(json({ disposition: 'FRESH_CACHE', overview: { tenantId, timezone: 'Asia/Bangkok', items: [] }, runs: [] }));
  });
  await page.route(`**${api}/viewer/tenants/${tenantId}/executive-overview/refreshes`, (route) => {
    refreshRequests++;
    expect(route.request().postDataJSON()).toEqual({ periodPreset: 'MONTH_TO_DATE', reportKeys: ['sales_goods_services', 'stock_balance'] });
    return route.fulfill(json({ id: refreshId, tenantId, status: 'QUEUED', total: 2, completed: 0, failed: 0, runs: [], createdAt: '2026-07-12T00:00:00Z' }, 202));
  });

  await page.goto(`/app/tenant/${tenantId}`);
  expect(automaticRevalidationRequests).toBe(0);
  await page.getByRole('button', { name: 'เปลี่ยนช่วง' }).click();
  await page.getByRole('button', { name: 'ดูภาพรวมช่วงนี้' }).click();
  await expect.poll(() => exactCacheLookupRequests).toBe(1);
  expect(refreshRequests).toBe(0);

  await page.getByRole('button', { name: 'เปลี่ยนช่วง' }).click();
  await page.getByRole('button', { name: 'ดึงใหม่จาก SML' }).click();

  await expect(page.getByRole('alertdialog')).toContainText('วาวา');
  await expect(page.getByRole('alertdialog')).toContainText('2 รายงาน');
  expect(refreshRequests).toBe(0);
  await page.getByRole('button', { name: 'ยกเลิก' }).click();
  expect(refreshRequests).toBe(0);

  await page.getByRole('button', { name: 'เปลี่ยนช่วง' }).click();
  await page.getByRole('button', { name: 'ดึงใหม่จาก SML' }).click();
  await page.goto(`/app/tenant/${tenantId}/report/sales_goods_services`);
  await expect(page.getByRole('alertdialog')).toHaveCount(0);
  expect(refreshRequests).toBe(0);

  await page.goto(`/app/tenant/${tenantId}`);
  await page.getByRole('button', { name: 'เปลี่ยนช่วง' }).click();
  await page.getByRole('button', { name: 'ดึงใหม่จาก SML' }).click();
  await page.getByRole('button', { name: 'ดึง SQL ใหม่' }).click();
  await expect.poll(() => refreshRequests).toBe(1);
});

test('exact overview refresh keeps partial results isolated and opens its run without another SQL request', async ({ page }) => {
  const refreshId = '44444444-4444-4444-8444-444444444444';
  const runId = '55555555-5555-4555-8555-555555555555';
  let createRunRequests = 0;
  const reports = [
    { reportKey: 'sales_goods_services', version: '1.0.0', label: 'รายงานขายสินค้าและบริการ', category: 'SALES', isSensitive: false, periodMode: 'DATE_RANGE' },
    { reportKey: 'stock_balance', version: '1.0.0', label: 'รายงานสต็อกคงเหลือ', category: 'INVENTORY', isSensitive: true, periodMode: 'AS_OF_DATE' }
  ];
  await page.route(`**${api}/viewer/me`, (route) => route.fulfill(json({ recipientId: '22222222-2222-4222-8222-222222222222', displayName: 'ผู้ทดสอบ', expiresAt: '2026-07-12T00:00:00Z' })));
  await page.route(`**${api}/viewer/tenants`, (route) => route.fulfill(json({ data: [{ id: tenantId, name: 'วาวา', timezone: 'Asia/Bangkok', reportKeys: reports.map((report) => report.reportKey) }], page: { hasMore: false } })));
  await page.route(`**${api}/viewer/tenants/${tenantId}/reports`, (route) => route.fulfill(json({ data: reports, page: { hasMore: false } })));
  await page.route(`**${api}/viewer/tenants/${tenantId}/executive-overview/refreshes/${refreshId}`, (route) => route.fulfill(json({
    id: refreshId, tenantId, status: 'PARTIAL', total: 2, completed: 1, failed: 1,
    runs: [{ reportKey: 'sales_goods_services', runId, status: 'SUCCEEDED' }, { reportKey: 'stock_balance', runId: '66666666-6666-4666-8666-666666666666', status: 'FAILED' }],
    createdAt: '2026-07-12T00:00:00Z', finishedAt: '2026-07-12T00:01:00Z'
  })));
  await page.route(`**${api}/viewer/tenants/${tenantId}/executive-overview/refreshes/${refreshId}/result`, (route) => route.fulfill(json({
    refreshId, tenantId, status: 'PARTIAL',
    items: [{ runId, dashboard: salesDashboard() }],
    failures: [{ reportKey: 'stock_balance', status: 'FAILED', safeErrorCode: 'SML_TIMEOUT' }]
  })));
  await page.route(`**${api}/viewer/tenants/${tenantId}/reports/sales_goods_services/runs**`, (route) => {
    if (route.request().method() === 'POST') { createRunRequests++; return route.fulfill(json({}, 500)); }
    if (route.request().url().endsWith('/dashboard')) return route.fulfill(json(salesDashboard()));
    return route.fulfill(json({ id: runId, tenantId, reportKey: 'sales_goods_services', status: 'SUCCEEDED', periodPreset: 'YESTERDAY', dateFrom: '2026-07-09', dateTo: '2026-07-09', rowCount: 1, isTruncated: false, queuedAt: '2026-07-10T00:00:00Z', finishedAt: '2026-07-10T00:00:01Z', expiresAt: '2026-07-11T00:00:00Z' }));
  });

  await page.goto(`/app/tenant/${tenantId}?refreshId=${refreshId}`);

  await expect(page.getByText('อัปเดตไม่สำเร็จ 1 รายงาน')).toContainText('รายงานสต็อกคงเหลือ');
  await expect(page.locator('.executive-kpi').filter({ hasText: 'มูลค่าสต็อก' })).toContainText('ยังไม่มีข้อมูล');
  await page.locator('.executive-kpi').filter({ hasText: 'ยอดขาย' }).click();
  await expect(page).toHaveURL(new RegExp(`runId=${runId}`));
  await expect(page.getByRole('tab', { name: 'ภาพรวมและกราฟ' })).toBeVisible();
  expect(createRunRequests).toBe(0);
});

test('LINE mobile overview renders readable responsive charts with full table data', async ({ page }) => {
  const consoleErrors = captureUnexpectedConsoleErrors(page);
  const reports = [
    ['sales_goods_services', 'รายงานขายสินค้าและบริการ'],
    ['gross_profit_by_product', 'กำไรขั้นต้นตามสินค้า'],
    ['stock_balance', 'รายงานสต็อกคงเหลือ'],
    ['cash_bank_receipts', 'รายงานรับเงิน']
  ] as const;
  await page.setViewportSize({ width: 390, height: 844 });
  await page.route(`**${api}/viewer/me`, (route) => route.fulfill(json({ recipientId: '22222222-2222-4222-8222-222222222222', displayName: 'ผู้ทดสอบ', expiresAt: '2026-07-12T00:00:00Z' })));
  await page.route(`**${api}/viewer/tenants`, (route) => route.fulfill(json({ data: [{ id: tenantId, name: 'ร้านตัวอย่าง', timezone: 'Asia/Bangkok', reportKeys: reports.map(([key]) => key) }], page: { hasMore: false } })));
  await page.route(`**${api}/viewer/tenants/${tenantId}/reports`, (route) => route.fulfill(json({ data: reports.map(([reportKey, label]) => ({ reportKey, label, version: '1.0.0', category: 'REPORT', isSensitive: false })), page: { hasMore: false } })));
  await page.route(`**${api}/viewer/tenants/${tenantId}/executive-overview`, (route) => route.fulfill(json({
      tenantId, timezone: 'Asia/Bangkok', items: reports.map(([reportKey], index) => ({
        runId: `${index + 1}1111111-1111-4111-8111-111111111111`, dashboard: dashboardForReport(reportKey),
        sourceFinishedAt: '2026-07-12T08:00:00+07:00', freshnessStatus: 'FRESH', detailsAvailable: false
      }))
  })));

  await page.goto(`/app/tenant/${tenantId}`);

  const firstKpi = await page.locator('.executive-kpi').first().boundingBox();
  expect(firstKpi?.y).toBeLessThanOrEqual(220);
  const stockCard = page.getByRole('article').filter({ has: page.getByRole('heading', { name: 'สินค้าที่มีมูลค่าคงเหลือสูงสุด' }) });
  await expect(stockCard.getByTestId('ranking-item')).toHaveCount(5);
  await expect(stockCard.getByTestId('ranking-item').filter({ hasText: 'สินค้าทดสอบชื่อยาวสำหรับผู้บริหาร อันดับ 1' })).toBeVisible();
  await expect(stockCard.locator('canvas')).toHaveCount(0);
  await stockCard.getByText('ดูครบทั้ง 10 รายการในรูปแบบตาราง').click();
  await expect(stockCard.locator('tbody tr')).toHaveCount(10);
  const hasHorizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
  expect(hasHorizontalOverflow).toBe(false);
  expect(consoleErrors).toEqual([]);
});

test('mobile report details use stacked rows without horizontal overflow', async ({ page }) => {
  const consoleErrors = captureUnexpectedConsoleErrors(page);
  const runId = '77777777-7777-4777-8777-777777777777';
  await page.setViewportSize({ width: 390, height: 844 });
  await page.route(`**${api}/viewer/me`, (route) => route.fulfill(json({
    recipientId: '22222222-2222-4222-8222-222222222222', displayName: 'ผู้ทดสอบ', expiresAt: '2026-07-11T00:00:00Z'
  })));
  await page.route(`**${api}/viewer/tenants`, (route) => route.fulfill(json({
    data: [{ id: tenantId, name: 'ร้านตัวอย่าง', timezone: 'Asia/Bangkok', reportKeys: ['sales_goods_services'] }],
    page: { hasMore: false }
  })));
  await page.route(`**${api}/viewer/tenants/${tenantId}/reports`, (route) => route.fulfill(json({
    data: [{ reportKey: 'sales_goods_services', version: '1.0.0', label: 'รายงานขายสินค้าและบริการ', category: 'SALES', isSensitive: false }],
    page: { hasMore: false }
  })));
  await page.route(`**${api}/viewer/tenants/${tenantId}/reports/sales_goods_services/snapshots/latest**`, (route) => route.fulfill(json({
    runId,
    dashboard: { ...salesDashboard(), period: { preset: 'MONTH_TO_DATE', dateFrom: '2026-07-01', dateTo: '2026-07-12' } },
    periodFrom: '2026-07-01', periodTo: '2026-07-12', sourceFinishedAt: '2026-07-10T00:00:01Z',
    freshnessStatus: 'FRESH', detailsAvailable: true, detailsExpiresAt: '2026-07-11T00:00:00Z'
  })));
  await page.route(`**${api}/viewer/tenants/${tenantId}/reports/sales_goods_services/runs**`, (route) => {
    if (route.request().url().includes('/rows')) {
      expect(route.request().method()).toBe('POST');
      expect(route.request().postDataJSON()).toEqual({ filters: [], page: 0, pageSize: 25 });
      return route.fulfill(json({
        runId, columns: ['doc_no', 'total_amount', 'last_status'], data: [{ doc_no: 'IV-001', total_amount: '1250.00', last_status: 'USER5' }],
        rowOrdinals: [1], filterCapabilities: [
          { columnKey: 'doc_no', dataType: 'IDENTIFIER', operators: ['CONTAINS', 'EQUALS'], globalSearchable: true },
          { columnKey: 'total_amount', dataType: 'NUMBER', operators: ['EQUALS', 'GTE', 'LTE'], globalSearchable: false }
        ], page: 0, pageSize: 25, total: 1, totalPages: 1
      }));
    }
    if (route.request().url().endsWith('/dashboard')) return route.fulfill(json(salesDashboard()));
    return route.fulfill(json({
      id: runId, tenantId, reportKey: 'sales_goods_services', status: 'SUCCEEDED', periodPreset: 'MONTH_TO_DATE',
      dateFrom: '2026-07-01', dateTo: '2026-07-12', rowCount: 1, isTruncated: false,
      summary: { document_count: '1', total_amount: '1250.00' }, queuedAt: '2026-07-10T00:00:00Z',
      finishedAt: '2026-07-10T00:00:01Z', expiresAt: '2026-07-11T00:00:00Z'
    }, route.request().method() === 'POST' ? 201 : 200));
  });

  await page.goto(`/app/tenant/${tenantId}/report/sales_goods_services`);

  await expect(page.getByRole('tab', { name: 'ข้อมูลรายละเอียด' })).toBeVisible();
  await page.getByRole('tab', { name: 'ข้อมูลรายละเอียด' }).click();
  const mobileDetails = page.getByLabel('รายละเอียดรายงานแบบมือถือ');
  await expect(mobileDetails).toBeVisible();
  await expect(mobileDetails.getByText('IV-001')).toBeVisible();
  await expect(mobileDetails.getByText('1,250')).toBeVisible();
  await expect(mobileDetails.getByText('USER5')).toHaveCount(0);
  await page.getByLabel('เลือกคอลัมน์ที่ต้องการแสดง').focus();
  await page.keyboard.press('ArrowDown');
  await page.getByRole('option').filter({ hasText: 'สถานะภายใน' }).click();
  await page.keyboard.press('Escape');
  await mobileDetails.getByRole('button', { name: 'ดูรายละเอียดเพิ่ม' }).click();
  await expect(mobileDetails.getByText('USER5')).toBeVisible();
  const hasHorizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
  expect(hasHorizontalOverflow).toBe(false);
  expect(consoleErrors).toEqual([]);
});

test('LINE deep link opens the exact snapshot without creating a new SQL run', async ({ page }) => {
  const consoleErrors = captureUnexpectedConsoleErrors(page);
  const runId = '77777777-7777-4777-8777-777777777777';
  const deliveryId = '88888888-8888-4888-8888-888888888888';
  let createRunRequests = 0;
  await page.route(`**${api}/viewer/me`, (route) => route.fulfill(json({
    recipientId: '22222222-2222-4222-8222-222222222222', displayName: 'ผู้ทดสอบ', expiresAt: '2026-07-11T00:00:00Z'
  })));
  await page.route(`**${api}/viewer/tenants`, (route) => route.fulfill(json({
    data: [{ id: tenantId, name: 'ร้านตัวอย่าง', timezone: 'Asia/Bangkok', reportKeys: ['sales_goods_services'] }], page: { hasMore: false }
  })));
  await page.route(`**${api}/viewer/tenants/${tenantId}/reports`, (route) => route.fulfill(json({
    data: [{ reportKey: 'sales_goods_services', version: '1.0.0', label: 'รายงานขายสินค้าและบริการ', category: 'SALES', isSensitive: false }], page: { hasMore: false }
  })));
  await page.route(`**${api}/viewer/delivery-contexts`, (route) => route.fulfill(json({
    deliveryId, tenantId, scheduledFor: '2026-07-10T01:00:00Z', materializationVersion: 2,
    orderStatus: 'EXACT', dataStatus: 'AVAILABLE', reports: [{
      reportKey: 'sales_goods_services', label: 'รายงานขายสินค้าและบริการ', position: 1,
      reportRunId: runId, snapshotStatus: 'DETAIL_EXPIRED', summary: {
        dashboard: salesDashboard(), sourceConsistency: 'STATEMENT',
        sourceStartedAt: '2026-07-10T00:00:00Z', sourceFinishedAt: '2026-07-10T00:00:01Z'
      }
    }]
  })));
  await page.route(`**${api}/viewer/tenants/${tenantId}/reports/sales_goods_services/runs**`, (route) => {
    if (route.request().method() === 'POST') {
      createRunRequests++;
      return route.fulfill(json({}, 500));
    }
    if (route.request().url().endsWith('/dashboard')) return route.fulfill(json(salesDashboard()));
    return route.fulfill(json({
      id: runId, tenantId, reportKey: 'sales_goods_services', status: 'EXPIRED', periodPreset: 'YESTERDAY',
      dateFrom: '2026-07-09', dateTo: '2026-07-09', rowCount: 741, isTruncated: false,
      summary: {}, queuedAt: '2026-07-10T00:00:00Z', finishedAt: '2026-07-10T00:00:01Z', expiresAt: '2026-07-11T00:00:00Z'
    }));
  });

  await page.goto(`/app/tenant/${tenantId}/report/sales_goods_services?snapshotRunId=${runId}&deliveryRef=${'d'.repeat(32)}`);

  await expect(page).toHaveURL(`/app/tenant/${tenantId}/delivery/${deliveryId}/report/sales_goods_services`);
  await expect(page.getByText('ข้อมูลจาก LINE', { exact: true })).toBeVisible();
  await expect(page.getByText('฿1,250')).toBeVisible();
  expect(createRunRequests).toBe(0);
  await expect(page.getByText('ข้อมูลแถวรายละเอียดหมดอายุแล้ว')).toBeVisible();
  expect(consoleErrors).toEqual([]);
});

test('admin edits a schedule on a full page and previews the exact single Flex card', async ({ page }) => {
  const consoleErrors = captureUnexpectedConsoleErrors(page, [404]);
  const session = { username: 'superadmin', expiresAt: '2026-07-11T00:00:00Z', mustRotateBootstrapPassword: false };
  const recipientId = '33333333-3333-4333-8333-333333333333';
  let previewRequests = 0;
  let testSendRequests = 0;
  let tenantPatchRequests = 0;
  let archiveRequests = 0;
  let restoreRequests = 0;

  await page.route(`**${api}/auth/admin/session`, (route) => route.fulfill(json(session)));
  await page.route(`**${api}/admin/reports`, (route) => route.fulfill(json(adminReportCatalog)));
  await page.route(`**${api}/admin/tenants/${tenantId}`, (route) => {
    if (route.request().method() === 'PATCH') tenantPatchRequests++;
    return route.fulfill(json({
      id: tenantId, slug: 'sample-shop', name: 'ร้านตัวอย่าง', timezone: 'Asia/Bangkok',
      status: route.request().method() === 'PATCH' ? 'DISABLED' : 'ACTIVE',
      accessEndsAt: '2027-07-10T00:00:00Z', version: route.request().method() === 'PATCH' ? 2 : 1, smlReadiness: 'READY',
      createdAt: '2026-07-01T00:00:00Z', updatedAt: '2026-07-10T00:00:00Z'
    }));
  });
  await page.route(`**${api}/admin/tenants/${tenantId}/sml-connection`, (route) => route.fulfill(json({ error: { code: 'SML_NOT_CONFIGURED', message: 'Not configured', requestId: 'e2e', retryable: false } }, 404)));
  const recipientRows = [{ id: recipientId, status: 'ACTIVE', displayName: 'เจ้าของร้าน', reportKeys: ['sales_goods_services'], permissionsVersion: 1, createdAt: '2026-07-01T00:00:00Z' }];
  await page.route(`**${api}/admin/tenants/${tenantId}/recipients**`, (route) => route.fulfill(json(isTableQuery(route.request().url()) ? { ...tableResult(recipientRows), selected: [] } : { data: recipientRows, page: { hasMore: false } })));
  await page.route(`**${api}/admin/tenants/${tenantId}/schedule-recipient-options/query`, (route) => route.fulfill(json({
    data: [{ id: recipientId, status: 'ACTIVE', displayName: 'เจ้าของร้าน', eligible: true, missingReportKeys: [] }],
    selected: [{ id: recipientId, status: 'ACTIVE', displayName: 'เจ้าของร้าน', eligible: true, missingReportKeys: [] }],
    page: 0, pageSize: 100, total: 1
  })));
  const schedule = {
    id: '44444444-4444-4444-8444-444444444444', tenantId, name: 'Morning', daysOfWeek: [1, 2, 3, 4, 5],
    localTime: '09:00', timezone: 'Asia/Bangkok', periodPreset: 'YESTERDAY',
    reportKeys: ['sales_goods_services'], recipientIds: [recipientId], status: 'DRAFT', version: 1,
    readinessBlockers: [], nextOccurrences: ['2026-07-13T02:00:00Z'],
    createdAt: '2026-07-01T00:00:00Z', updatedAt: '2026-07-10T00:00:00Z'
  };
  let scheduleStatus = 'DRAFT';
  let scheduleVersion = 1;
  await page.route(`**${api}/admin/tenants/${tenantId}/schedules**`, async (route) => {
    if (route.request().url().endsWith('/preview')) {
      previewRequests++;
      expect(route.request().postDataJSON()).toEqual({
        daysOfWeek: [1, 2, 3, 4, 5], localTime: '09:00', timezone: 'Asia/Bangkok',
        periodPreset: 'YESTERDAY', reportKeys: ['sales_goods_services']
      });
      await route.fulfill(json({
        presentationVersion: 'executive-navy-v2', exampleScheduledFor: '2026-07-13T09:00:00+07:00', mixedPeriods: false,
        altText: 'รายงาน ร้านตัวอย่าง — ข้อมูลวันที่ 2026-07-10', tenantName: 'ร้านตัวอย่าง',
        period: { preset: 'YESTERDAY', dateFrom: '2026-07-10', dateTo: '2026-07-10' },
        periodLabel: 'ข้อมูลวันที่ 2026-07-10', generatedAt: '2026-07-11T01:30:00+07:00',
        actionUrl: 'https://dashboard.nextstep-soft.com/app', payloadBytes: 2048, message: {},
        reports: [{ key: 'sales_goods_services', label: 'รายงานขายสินค้าและบริการ', categoryLabel: 'ขาย', periodLabel: 'ข้อมูลวันที่ 2026-07-10', dataState: 'DATA',
          primary: { label: 'ยอดขาย', value: '1,234,567.89' }, supporting: [{ label: 'จำนวนเอกสาร', value: '128' }], metrics: [
          { label: 'เอกสาร', value: '128' }, { label: 'ยอดขาย', value: '1,234,567.89' }
        ] }]
      }));
      return;
    }
    if (route.request().url().endsWith('/test-send')) {
      testSendRequests++;
      await route.fulfill(json({
        id: '55555555-5555-4555-8555-555555555555', tenantId, scheduleId: '44444444-4444-4444-8444-444444444444',
        scheduledFor: '2026-07-10T12:00:00Z', status: 'COLLECTING',
        reportRunIds: ['66666666-6666-4666-8666-666666666666']
      }, 202));
      return;
    }
    if (route.request().method() === 'DELETE') {
      archiveRequests++;
      expect(route.request().url()).toContain('version=1');
      scheduleStatus = 'ARCHIVED';
      scheduleVersion = 2;
      await route.fulfill(json({ ...schedule, status: scheduleStatus, version: scheduleVersion, archivedAt: '2026-07-11T12:00:00Z', nextOccurrences: [], readinessBlockers: [] }));
      return;
    }
    if (route.request().method() === 'POST' && route.request().url().includes('/restore')) {
      restoreRequests++;
      expect(route.request().url()).toContain('version=2');
      scheduleStatus = 'DRAFT';
      scheduleVersion = 3;
      await route.fulfill(json({ ...schedule, status: scheduleStatus, version: scheduleVersion }));
      return;
    }
    if (route.request().method() === 'GET' && route.request().url().endsWith(`/schedules/${schedule.id}`)) {
      await route.fulfill(json({ ...schedule, status: scheduleStatus, version: scheduleVersion }));
      return;
    }
    if (isTableQuery(route.request().url())) {
      const input = route.request().postDataJSON() as { filters?: { includeArchived?: boolean } };
      const includeArchived = input.filters?.includeArchived === true;
      const rows = scheduleStatus === 'ARCHIVED' && !includeArchived ? [] : [{ ...schedule, status: scheduleStatus, version: scheduleVersion, ...(scheduleStatus === 'ARCHIVED' ? { archivedAt: '2026-07-11T12:00:00Z', nextOccurrences: [], readinessBlockers: [] } : {}) }];
      await route.fulfill(json(tableResult(rows)));
      return;
    }
    const includeArchived = new URL(route.request().url()).searchParams.get('includeArchived') === 'true';
    await route.fulfill(json({
      data: scheduleStatus === 'ARCHIVED' && !includeArchived ? [] : [{ ...schedule, status: scheduleStatus, version: scheduleVersion, ...(scheduleStatus === 'ARCHIVED' ? { archivedAt: '2026-07-11T12:00:00Z', nextOccurrences: [], readinessBlockers: [] } : {}) }],
      page: { hasMore: false }
    }));
  });

  await page.goto(`/admin/tenants/${tenantId}`);
  await page.getByRole('tab', { name: 'การเชื่อมต่อ SML' }).click();
  await page.getByLabel('ชื่อฐานข้อมูล SML').fill('CHANGED_DATA');
  await expect(page.getByRole('button', { name: 'บันทึกการเชื่อมต่อ' })).toBeEnabled();
  await expect(page.getByRole('button', { name: 'ทดสอบการเชื่อมต่อ' })).toBeDisabled();
  await expect(page.getByText('บันทึกค่าก่อนทดสอบการเชื่อมต่อ')).toBeVisible();
  await page.getByRole('tab', { name: 'ตารางส่ง LINE' }).click();
  page.once('dialog', (dialog) => dialog.accept());
  await page.getByRole('button', { name: 'แก้ไข' }).click();
  await expect(page).toHaveURL(`/admin/tenants/${tenantId}/schedules/${schedule.id}/edit`);
  await expect(page.getByRole('link', { name: 'ร้านค้า', exact: true })).toHaveClass(/active-route/);
  await expect(page.getByRole('heading', { name: 'แก้ไขตารางส่งรายงาน' })).toBeVisible();
  await expect(page.getByText('1 LINE Card · 1/10 รายงาน')).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  for (const name of ['เลื่อน รายงานขายสินค้าและบริการ ขึ้น', 'ยกเลิก', 'ดูตัวอย่าง LINE Card', 'บันทึกตารางส่ง']) {
    const box = await page.getByRole('button', { name }).boundingBox();
    expect(box?.height).toBeGreaterThanOrEqual(44);
    expect(box?.width).toBeGreaterThanOrEqual(44);
  }
  await page.getByRole('button', { name: 'ดูตัวอย่าง LINE Card' }).click();

  await expect(page.getByLabel('ตัวอย่าง LINE Flex Message')).toBeVisible();
  await expect(page.getByText('ตัวเลขสมมติเท่านั้น')).toBeVisible();
  await expect(page.getByText('1,234,567.89')).toBeVisible();
  await expect(page.getByText('ดูภาพรวมร้าน')).toBeVisible();
  await expect(page.locator('.flex-preview-card')).toHaveAttribute('data-presentation-version', 'executive-navy-v2');
  await expect(page.getByText('1 LINE Card · 1/10 รายงาน · 2.0 KB / 30 KB')).toBeVisible();
  expect(previewRequests).toBe(1);
  await page.getByRole('button', { name: 'ตารางส่งรายงาน' }).click();
  await expect(page).toHaveURL(new RegExp(`/admin/tenants/${tenantId}\\?tab=schedules`));
  await page.setViewportSize({ width: 1280, height: 720 });

  await page.getByRole('button', { name: 'ทดสอบส่ง LINE' }).click();
  await expect(page.getByText('ยืนยันส่ง LINE จริง')).toBeVisible();
  await expect(page.getByText('ใช้ quota ของ OA กลาง')).toBeVisible();
  expect(testSendRequests).toBe(0);
  await page.getByRole('button', { name: 'ดึงข้อมูลและส่งจริง' }).click();
  await expect(page.getByText('รับคำขอทดสอบส่งแล้ว')).toBeVisible();
  expect(testSendRequests).toBe(1);

  await page.getByRole('button', { name: 'ลบตารางส่งรายงาน' }).click();
  await expect(page.getByText('ยืนยันลบตารางส่งรายงาน')).toBeVisible();
  await expect(page.getByText('ประวัติการส่งเดิมยังเก็บไว้ 365 วัน')).toBeVisible();
  await page.getByRole('button', { name: 'ลบตารางส่งรายงาน', exact: true }).last().click();
  await expect(page.getByText('ลบตารางส่งรายงานแล้ว')).toBeVisible();
  expect(archiveRequests).toBe(1);
  await page.getByText('รวมรายการที่ลบแล้ว').click();
  await expect(page.getByText('ลบแล้ว', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'กู้คืนเป็นฉบับร่าง' }).click();
  await expect(page.getByText('กู้คืนตารางส่งรายงาน')).toBeVisible();
  await page.getByRole('button', { name: 'กู้คืนเป็นฉบับร่าง', exact: true }).last().click();
  await expect(page.getByText('กู้คืนเป็นฉบับร่างแล้ว')).toBeVisible();
  expect(restoreRequests).toBe(1);

  await page.getByRole('tab', { name: 'ข้อมูลร้าน' }).click();
  await page.getByLabel('สถานะ', { exact: true }).click();
  await page.getByRole('option', { name: 'DISABLED' }).click();
  await page.getByRole('button', { name: 'บันทึกข้อมูลร้าน' }).click();
  await expect(page.getByText('ยืนยันการเปลี่ยนสิทธิ์ร้าน')).toBeVisible();
  await expect(page.getByText('หยุดการเปิด Dashboard')).toBeVisible();
  expect(tenantPatchRequests).toBe(0);
  await page.getByRole('button', { name: 'ยืนยันและบันทึก' }).click();
  await expect(page.getByText('บันทึกร้านค้าแล้ว')).toBeVisible();
  expect(tenantPatchRequests).toBe(1);
  expect(consoleErrors).toEqual([]);
});

test('viewer opens all ten report routes with the shared executive layout', async ({ page }) => {
  let createRunRequests = 0;
  let revalidationRequests = 0;
  const reports = [
    ['sales_goods_services', 'รายงานขายสินค้าและบริการ'],
    ['purchase_goods_payables', 'รายงานซื้อสินค้าและตั้งหนี้'],
    ['gross_profit_by_product', 'กำไรขั้นต้นตามสินค้า'],
    ['gross_profit_by_ar_customer', 'กำไรขั้นต้นตามลูกหนี้'],
    ['stock_balance', 'รายงานสต็อกคงเหลือ'],
    ['stock_reorder', 'รายงานสินค้าถึงจุดสั่งซื้อ'],
    ['ar_customer_movement', 'รายงานความเคลื่อนไหวลูกหนี้'],
    ['ar_debt_receipt', 'รายงานรับชำระหนี้'],
    ['cash_bank_receipts', 'รายงานรับเงิน'],
    ['cash_bank_payments', 'รายงานจ่ายเงิน']
  ] as const;
  await page.route(`**${api}/viewer/me`, (route) => route.fulfill(json({ recipientId: '22222222-2222-4222-8222-222222222222', displayName: 'ผู้ทดสอบ', expiresAt: '2026-07-11T00:00:00Z' })));
  await page.route(`**${api}/viewer/tenants`, (route) => route.fulfill(json({ data: [{ id: tenantId, name: 'ร้านตัวอย่าง', timezone: 'Asia/Bangkok', reportKeys: reports.map(([key]) => key) }], page: { hasMore: false } })));
  await page.route(`**${api}/viewer/tenants/${tenantId}/reports`, (route) => route.fulfill(json({ data: reports.map(([reportKey, label]) => ({ reportKey, label, version: '1.0.0', category: 'REPORT', isSensitive: false })), page: { hasMore: false } })));
  const runIds = new Map(reports.map(([key], index) => [key, `${String(index + 1).padStart(8, '0')}-1111-4111-8111-111111111111`]));
  await page.route(`**${api}/viewer/tenants/${tenantId}/reports/*/snapshots/latest**`, (route) => {
    const reportKey = route.request().url().split('/reports/')[1]!.split('/')[0]!;
    return route.fulfill(json({
      runId: runIds.get(reportKey), dashboard: dashboardForReport(reportKey),
      periodFrom: '2026-07-09', periodTo: '2026-07-09', sourceFinishedAt: '2026-07-10T00:00:01Z',
      freshnessStatus: 'FRESH', detailsAvailable: true, detailsExpiresAt: '2026-07-11T00:00:00Z'
    }));
  });
  await page.route(`**${api}/viewer/tenants/${tenantId}/reports/*/revalidations`, (route) => {
    revalidationRequests++;
    return route.fulfill(json({ disposition: 'FRESH_CACHE' }));
  });
  await page.route(`**${api}/viewer/tenants/${tenantId}/reports/*/runs**`, (route) => {
    const reportKey = route.request().url().split('/reports/')[1]!.split('/')[0]!;
    if (route.request().method() === 'POST') createRunRequests++;
    if (route.request().url().endsWith('/dashboard')) return route.fulfill(json(dashboardForReport(reportKey)));
    return route.fulfill(json({ id: runIds.get(reportKey), tenantId, reportKey, status: 'SUCCEEDED', periodPreset: reportKey.startsWith('stock_') ? 'AS_OF_RUN' : 'YESTERDAY', dateFrom: '2026-07-09', dateTo: '2026-07-09', rowCount: 0, isTruncated: false, queuedAt: '2026-07-10T00:00:00Z', finishedAt: '2026-07-10T00:00:01Z', expiresAt: '2026-07-11T00:00:00Z' }, route.request().method() === 'POST' ? 202 : 200));
  });

  for (const width of [390, 1440]) {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 1000 });
    for (const [reportKey, label] of reports) {
      await page.goto(`/app/tenant/${tenantId}/report/${reportKey}`);
      if (width === 390) await expect(page.getByTestId('mobile-topbar-context')).toContainText(label);
      await expect(page.getByRole('heading', { name: label })).toBeVisible();
      await expect(page.getByRole('tab', { name: 'ภาพรวมและกราฟ' })).toBeVisible();
      await expect(page.getByText('เทียบกับ 8 ก.ค. 2569')).toBeVisible();
      if (width === 390) await expect(page.getByRole('button', { name: reportKey === 'stock_reorder' ? 'รีเฟรช' : 'เปลี่ยนช่วง' })).toBeVisible();
      else await expect(page.getByRole('button', { name: /ดูช่วงนี้|ดูสถานะล่าสุด/ })).toBeVisible();
      await expect(page.locator('.report-summary-bar')).toHaveCount(0);
      if (width === 1440) {
        const header = await page.locator('.page-header').boundingBox();
        const toolbar = await page.locator('.period-toolbar').boundingBox();
        const customPeriod = await page.locator('.period-toolbar').getAttribute('class').then((value) => value?.includes('has-custom-period') ?? false);
        expect(header?.height).toBeLessThanOrEqual(64);
        expect(toolbar?.height).toBeLessThanOrEqual(customPeriod ? 150 : 80);
        if (!customPeriod && reportKey !== 'stock_reorder') {
          const alignment = await page.evaluate(() => {
            const selectors = ['.period-context-value', '.period-preset-field .p-select', '.period-actions .p-button'];
            return selectors.map((selector) => {
              const rect = document.querySelector(selector)?.getBoundingClientRect();
              return rect ? rect.bottom : null;
            });
          });
          expect(alignment.every((value) => value !== null)).toBe(true);
          const bottoms = alignment.filter((value): value is number => value !== null);
          expect(Math.max(...bottoms) - Math.min(...bottoms)).toBeLessThanOrEqual(4);
        }
      }
      await expect(page.getByLabel('ตัวกรองรายงาน')).toHaveCount(0);
      const hasHorizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
      expect(hasHorizontalOverflow).toBe(false);
    }
  }
  expect(revalidationRequests).toBe(0);
  expect(createRunRequests).toBe(0);
});
