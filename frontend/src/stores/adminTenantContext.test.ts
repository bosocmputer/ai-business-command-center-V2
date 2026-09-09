import { beforeEach, describe, expect, it } from 'vitest';
import {
  beginAdminTenantContext,
  clearAdminTenantContext,
  resolveAdminMobileContext,
  setAdminTenantContext,
  useAdminTenantContext
} from './adminTenantContext';

describe('admin tenant context', () => {
  beforeEach(() => clearAdminTenantContext());

  it('clears the previous tenant synchronously and ignores a stale response', () => {
    beginAdminTenantContext('tenant-a');
    setAdminTenantContext('tenant-a', 'ร้าน A');
    expect(useAdminTenantContext().state.name).toBe('ร้าน A');

    beginAdminTenantContext('tenant-b');
    expect(useAdminTenantContext().state.name).toBe('');

    setAdminTenantContext('tenant-a', 'ร้าน A ตอบช้า');
    expect(useAdminTenantContext().state.name).toBe('');

    setAdminTenantContext('tenant-b', 'ร้าน B');
    expect(useAdminTenantContext().state.name).toBe('ร้าน B');
  });

  it('uses the resolved tenant as mobile context only when the route matches', () => {
    beginAdminTenantContext('tenant-a');
    setAdminTenantContext('tenant-a', 'ร้าน A');

    expect(resolveAdminMobileContext('tenant-a', 'รายละเอียดร้าน')).toEqual({
      title: 'ร้าน A', subtitle: 'รายละเอียดร้าน', homeTo: '/admin/tenants/tenant-a'
    });
    expect(resolveAdminMobileContext('tenant-b', 'รายละเอียดร้าน')).toEqual({
      title: 'รายละเอียดร้าน', subtitle: 'Nextstep Admin', homeTo: '/admin'
    });
  });
});
