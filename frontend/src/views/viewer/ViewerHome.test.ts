import { mount } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  push: vi.fn(async () => undefined),
  state: {
    tenants: [
      { id: 'tenant-krabi', name: 'ร้าน กระบี่นางทอง', timezone: 'Asia/Bangkok', reportKeys: [] }
    ]
  }
}));

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: mocks.push })
}));

vi.mock('@/stores/viewer', () => ({
  useViewerSession: () => ({ state: mocks.state })
}));

import ViewerHome from './ViewerHome.vue';

describe('ViewerHome tenant chooser', () => {
  it('shows an active tenant before report permissions are assigned', async () => {
    const wrapper = mount(ViewerHome, {
      global: {
        stubs: {
          Button: {
            props: ['ariaLabel'],
            emits: ['click'],
            template: '<button :aria-label="ariaLabel" @click="$emit(\'click\')"><slot /></button>'
          },
          Tag: { props: ['value'], template: '<span>{{ value }}</span>' },
          IconField: true,
          InputIcon: true,
          InputText: true,
          Message: true
        }
      }
    });

    expect(wrapper.text()).toContain('ร้าน กระบี่นางทอง');
    expect(wrapper.text()).toContain('เข้าร่วมร้านแล้ว');
    expect(wrapper.text()).toContain('ยังไม่ได้กำหนดสิทธิ์รายงาน');
    expect(wrapper.get('button').attributes('aria-label')).toContain('ยังไม่ได้กำหนดสิทธิ์รายงาน');

    await wrapper.get('button').trigger('click');
    expect(mocks.push).toHaveBeenCalledWith('/app/tenant/tenant-krabi');
  });
});
