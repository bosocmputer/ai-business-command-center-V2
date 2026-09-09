import { flushPromises, mount } from '@vue/test-utils';
import { afterEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => {
  const state = {
    me: { recipientId: 'existing-recipient', displayName: 'บอส เฉย ๆ', expiresAt: '2026-07-15T00:00:00Z' },
    tenants: [] as Array<{ id: string; name: string; timezone: string; reportKeys: [] }>,
    selectedTenantId: '',
    reportsByTenant: {},
    deliveryContexts: {},
    ready: true
  };
  return {
    state,
    route: { name: 'viewer-invite', path: '/app/invite', params: {}, query: { ref: 'i'.repeat(32) }, hash: '' },
    replace: vi.fn(async () => undefined),
    push: vi.fn(async () => undefined),
    loadViewer: vi.fn(async () => undefined),
    ensureReports: vi.fn(async () => []),
    exchange: vi.fn(async () => ({ recipientId: 'boss-recipient', displayName: 'บอส เฉย ๆ', csrfToken: 'csrf', expiresAt: '2026-07-15T00:00:00Z' })),
    logout: vi.fn(async () => undefined),
    tenants: vi.fn(async () => ({ data: [
      { id: 'tenant-wawa', name: 'วาวา', timezone: 'Asia/Bangkok', reportKeys: [] },
      { id: 'tenant-krabi', name: 'กระบี่', timezone: 'Asia/Bangkok', reportKeys: [] }
    ] })),
    liff: {
      init: vi.fn(async () => undefined),
      isLoggedIn: vi.fn(() => true),
      isInClient: vi.fn(() => false),
      getIDToken: vi.fn(() => 'opaque-line-id-token-that-is-long-enough'),
      login: vi.fn(),
      logout: vi.fn()
    }
  };
});

vi.mock('vue-router', () => ({
  useRoute: () => mocks.route,
  useRouter: () => ({ replace: mocks.replace, push: mocks.push })
}));

vi.mock('primevue/usetoast', () => ({ useToast: () => ({ add: vi.fn() }) }));
vi.mock('@line/liff', () => ({ default: mocks.liff }));
vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>();
  return { ...actual, viewerApi: { ...actual.viewerApi, exchange: mocks.exchange, logout: mocks.logout, tenants: mocks.tenants } };
});
vi.mock('@/stores/viewer', () => ({
  useViewerSession: () => ({
    state: mocks.state,
    loadViewer: mocks.loadViewer,
    setViewer: vi.fn((me) => { mocks.state.me = me; mocks.state.ready = true; }),
    setTenants: vi.fn((tenants) => { mocks.state.tenants = tenants; }),
    selectTenant: vi.fn(),
    setReports: vi.fn(),
    setDeliveryContext: vi.fn(),
    ensureReports: mocks.ensureReports,
    clearViewer: vi.fn()
  })
}));

import ViewerShell from './ViewerShell.vue';

describe('ViewerShell invitation authentication', () => {
  it('redeems the invitation carried by liff.state instead of reusing an existing viewer session', async () => {
    vi.stubEnv('VITE_LINE_LIFF_ID', '2010662588-testLiff');
    const invitationReference = 'l'.repeat(32);
    Object.assign(mocks.route, {
      name: 'viewer-home',
      path: '/app',
      params: {},
      query: {
        'liff.state': `/invite?ref=${invitationReference}`,
        code: 'transient-oauth-code',
        state: 'transient-oauth-state',
        liffClientId: '2010662588'
      },
      hash: ''
    });

    const wrapper = mount(ViewerShell, {
      global: {
        stubs: {
          AppShell: { template: '<main><slot /></main>' },
          RouterView: true,
          Button: true,
          ProgressSpinner: true,
          Skeleton: true
        }
      }
    });

    await flushPromises();

    expect(mocks.loadViewer).not.toHaveBeenCalled();
    expect(mocks.exchange).toHaveBeenCalledWith(
      'opaque-line-id-token-that-is-long-enough',
      invitationReference,
      undefined,
      undefined
    );
    expect(mocks.replace).toHaveBeenCalledWith({ path: '/app' });
    wrapper.unmount();
  });

  it('exchanges the invitation with LINE even when an existing viewer session is ready', async () => {
    vi.stubEnv('VITE_LINE_LIFF_ID', '2010662588-testLiff');
    const wrapper = mount(ViewerShell, {
      global: {
        stubs: {
          AppShell: { template: '<main><slot /></main>' },
          RouterView: true,
          Button: true,
          ProgressSpinner: true,
          Skeleton: true
        }
      }
    });

    await flushPromises();

    expect(mocks.loadViewer).not.toHaveBeenCalled();
    expect(mocks.exchange).toHaveBeenCalledWith(
      'opaque-line-id-token-that-is-long-enough',
      'i'.repeat(32),
      undefined,
      undefined
    );
    expect(mocks.replace).toHaveBeenCalledWith({ path: '/app' });
    wrapper.unmount();
  });

  it('stays signed out instead of immediately starting LINE authentication again', async () => {
    vi.stubEnv('VITE_LINE_LIFF_ID', '2010662588-testLiff');
    const tenantId = 'a904bc92-a89b-463b-bc2a-565f09cbef44';
    Object.assign(mocks.route, {
      name: 'viewer-overview',
      path: `/app/tenant/${tenantId}`,
      params: { tenantId },
      query: {},
      hash: ''
    });
    mocks.state.tenants = [
      { id: tenantId, name: 'วาวา', timezone: 'Asia/Bangkok', reportKeys: [] }
    ];
    mocks.state.selectedTenantId = tenantId;

    const wrapper = mount(ViewerShell, {
      global: {
        stubs: {
          AppShell: {
            emits: ['signOut'],
            template: '<main><button data-testid="sign-out" @click="$emit(\'signOut\')">ออกจากระบบ</button><slot /></main>'
          },
          RouterView: true,
          Button: true,
          ProgressSpinner: true,
          Skeleton: true
        }
      }
    });
    await flushPromises();

    mocks.loadViewer.mockClear();
    mocks.exchange.mockClear();
    mocks.liff.init.mockClear();
    await wrapper.get('[data-testid="sign-out"]').trigger('click');
    await flushPromises();

    expect(mocks.logout).toHaveBeenCalledTimes(1);
    expect(mocks.replace).toHaveBeenCalledWith('/app');
    expect(mocks.loadViewer).not.toHaveBeenCalled();
    expect(mocks.exchange).not.toHaveBeenCalled();
    expect(mocks.liff.init).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain('ออกจากระบบแล้ว');
    wrapper.unmount();
  });

  it('opens a tenant overview without requesting the report catalog when no reports are assigned', async () => {
    vi.stubEnv('VITE_LINE_LIFF_ID', '2010662588-testLiff');
    const tenantId = '88bfcb51-73fe-469a-964a-675e6386c644';
    Object.assign(mocks.route, {
      name: 'viewer-overview',
      path: `/app/tenant/${tenantId}`,
      params: { tenantId },
      query: {},
      hash: ''
    });
    mocks.state.tenants = [
      { id: tenantId, name: 'ร้าน กระบี่นางทอง', timezone: 'Asia/Bangkok', reportKeys: [] }
    ];

    const wrapper = mount(ViewerShell, {
      global: {
        stubs: {
          AppShell: { template: '<main><slot /></main>' },
          RouterView: true,
          Button: true,
          ProgressSpinner: true,
          Skeleton: true
        }
      }
    });
    await flushPromises();

    expect(mocks.ensureReports).not.toHaveBeenCalled();
    expect(wrapper.text()).not.toContain('ไม่สามารถเปิด Dashboard');
    wrapper.unmount();
  });

  it('rejects a direct report route when the tenant has no permission for that report', async () => {
    vi.stubEnv('VITE_LINE_LIFF_ID', '2010662588-testLiff');
    const tenantId = '88bfcb51-73fe-469a-964a-675e6386c644';
    Object.assign(mocks.route, {
      name: 'viewer-report',
      path: `/app/tenant/${tenantId}/report/sales_goods_services`,
      params: { tenantId, reportKey: 'sales_goods_services' },
      query: {},
      hash: ''
    });
    mocks.state.tenants = [
      { id: tenantId, name: 'ร้าน กระบี่นางทอง', timezone: 'Asia/Bangkok', reportKeys: [] }
    ];

    const wrapper = mount(ViewerShell, {
      global: {
        stubs: {
          AppShell: { template: '<main><slot /></main>' },
          RouterView: true,
          Button: true,
          ProgressSpinner: true,
          Skeleton: true
        }
      }
    });
    await flushPromises();

    expect(wrapper.text()).toContain('ไม่สามารถเปิด Dashboard');
    expect(wrapper.text()).toContain('ไม่สามารถเปิดร้านหรือไม่มีสิทธิ์ใช้งาน');
    expect(mocks.ensureReports).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});

afterEach(() => {
  vi.clearAllMocks();
  vi.unstubAllEnvs();
  sessionStorage.clear();
  mocks.state.tenants = [];
  mocks.state.selectedTenantId = '';
  mocks.state.ready = true;
  mocks.ensureReports.mockReset();
  mocks.ensureReports.mockResolvedValue([]);
  Object.assign(mocks.route, { name: 'viewer-invite', path: '/app/invite', params: {}, query: { ref: 'i'.repeat(32) }, hash: '' });
});
