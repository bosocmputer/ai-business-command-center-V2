import { describe, expect, it } from 'vitest';
import { hasAdminLoginInput, hasMinimumAdminPasswordLength } from './passwordPolicy';

describe('admin password policy', () => {
  it('allows any non-empty password to be submitted for verification', () => {
    expect(hasAdminLoginInput('superadmin', 'x')).toBe(true);
    expect(hasAdminLoginInput('superadmin', '')).toBe(false);
    expect(hasAdminLoginInput('   ', 'x')).toBe(false);
  });

  it('requires 15 Unicode characters only when setting a new password', () => {
    expect(hasMinimumAdminPasswordLength('12345678901234')).toBe(false);
    expect(hasMinimumAdminPasswordLength('123456789012345')).toBe(true);
    expect(hasMinimumAdminPasswordLength('ก'.repeat(15))).toBe(true);
  });
});
