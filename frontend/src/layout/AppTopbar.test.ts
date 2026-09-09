import { mount } from '@vue/test-utils';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useLayout } from '@/layout/composables/layout';
import AppTopbar from './AppTopbar.vue';

beforeEach(() => Object.defineProperty(window, 'innerWidth', { configurable: true, value: 390 }));
afterEach(() => {
  const { layoutState } = useLayout();
  layoutState.mobileMenuActive = false;
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1024 });
});

describe('AppTopbar contextual mobile presentation', () => {
  it('shows the tenant and current page as the mobile context while preserving the desktop brand', () => {
    const wrapper = mount(AppTopbar, {
      props: {
        homeTo: '/app/tenant/tenant-1',
        mobileHomeTo: '/app/tenant/tenant-1',
        mobileTitle: 'วาวา',
        mobileSubtitle: 'ภาพรวม'
      },
      global: { stubs: { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } } }
    });

    expect(wrapper.get('[data-testid="mobile-topbar-context"]').text()).toContain('วาวา');
    expect(wrapper.get('[data-testid="mobile-topbar-context"]').text()).toContain('ภาพรวม');
    expect(wrapper.get('.brand-name').text()).toBe('NEXTSTEP');
    expect(wrapper.get('[aria-label="เปิดหรือปิดเมนู"]').attributes('aria-expanded')).toBe('false');
  });

  it('exposes the open drawer state to assistive technology', async () => {
    const wrapper = mount(AppTopbar, {
      props: { homeTo: '/admin', mobileTitle: 'ร้านค้า', mobileSubtitle: 'Nextstep Admin' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
    });

    await wrapper.get('[aria-label="เปิดหรือปิดเมนู"]').trigger('click');

    expect(wrapper.get('[aria-label="เปิดหรือปิดเมนู"]').attributes('aria-expanded')).toBe('true');
    expect(wrapper.get('[aria-label="เปิดหรือปิดเมนู"]').attributes('aria-controls')).toBe('app-sidebar');
  });
});
