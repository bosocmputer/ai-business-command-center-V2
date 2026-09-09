import { flushPromises, mount } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  tenantId: '88bfcb51-73fe-469a-964a-675e6386c644',
  ensureReports: vi.fn(async () => []),
  selectTenant: vi.fn()
}));

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { tenantId: mocks.tenantId }, query: {} }),
  useRouter: () => ({ replace: vi.fn(async () => undefined), push: vi.fn(async () => undefined) })
}));

vi.mock('primevue/useconfirm', () => ({
  useConfirm: () => ({ require: vi.fn(), close: vi.fn() })
}));
vi.mock('primevue/usetoast', () => ({ useToast: () => ({ add: vi.fn() }) }));
vi.mock('@/stores/viewer', () => ({
  useViewerSession: () => ({
    state: {
      tenants: [{ id: mocks.tenantId, name: 'ร้าน กระบี่นางทอง', timezone: 'Asia/Bangkok', reportKeys: [] }],
      reportsByTenant: {},
      selectedTenantId: mocks.tenantId
    },
    ensureReports: mocks.ensureReports,
    selectTenant: mocks.selectTenant,
    periodSelection: vi.fn(() => ({ periodPreset: 'MONTH_TO_DATE' })),
    setPeriodSelection: vi.fn()
  })
}));

import ExecutiveOverview from './ExecutiveOverview.vue';

describe('ExecutiveOverview without report permissions', () => {
  it('confirms tenant membership without loading reports or SML-backed data', async () => {
    const wrapper = mount(ExecutiveOverview, {
      global: {
        stubs: {
          AppPageHeader: { props: ['title'], template: '<h1>{{ title }}</h1>' },
          Tag: { props: ['value'], template: '<span>{{ value }}</span>' },
          ReportPeriodToolbar: true,
          Message: true,
          Button: true,
          Skeleton: true,
          ProgressBar: true,
          ExecutiveChart: true
        }
      }
    });
    await flushPromises();

    expect(wrapper.text()).toContain('ภาพรวมร้าน ร้าน กระบี่นางทอง');
    expect(wrapper.text()).toContain('เข้าร่วมร้านนี้เรียบร้อยแล้ว');
    expect(wrapper.text()).toContain('ยังไม่ได้กำหนดสิทธิ์รายงาน');
    expect(wrapper.text()).toContain('ไม่ต้องเปิดลิงก์เชิญหรือเข้าสู่ระบบใหม่');
    expect(mocks.ensureReports).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});
