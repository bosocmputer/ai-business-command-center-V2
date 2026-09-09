import { reactive, readonly } from 'vue';

type AdminTenantContextState = {
  tenantId: string;
  name: string;
};

type AdminMobileContext = {
  title: string;
  subtitle: string;
  homeTo: string;
};

const state = reactive<AdminTenantContextState>({ tenantId: '', name: '' });

export function beginAdminTenantContext(tenantId: string): void {
  if (state.tenantId === tenantId) return;
  state.tenantId = tenantId;
  state.name = '';
}

export function setAdminTenantContext(tenantId: string, name: string): void {
  if (state.tenantId !== tenantId) return;
  state.name = name.trim();
}

export function clearAdminTenantContext(): void {
  state.tenantId = '';
  state.name = '';
}

export function resolveAdminMobileContext(tenantId: string, pageTitle: string): AdminMobileContext {
  if (tenantId && state.tenantId === tenantId && state.name) {
    return { title: state.name, subtitle: pageTitle, homeTo: `/admin/tenants/${tenantId}` };
  }
  return { title: pageTitle, subtitle: 'Nextstep Admin', homeTo: '/admin' };
}

export function useAdminTenantContext() {
  return { state: readonly(state) };
}
