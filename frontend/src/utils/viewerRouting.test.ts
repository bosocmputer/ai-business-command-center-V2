import { describe, expect, it } from 'vitest';
import {
  deliveryContextRoute,
  explicitViewerTenantId,
  parseViewerEntryReferences,
  resolveViewerHome,
  resolveViewerNavigationReportKeys,
  resolveViewerTenantRoute,
  viewerNavigationScope
} from './viewerRouting';

const wawa = 'a904bc92-a89b-463b-bc2a-565f09cbef44';
const krabi = '88bfcb51-73fe-469a-964a-675e6386c644';

describe('resolveViewerTenantRoute', () => {
  it('uses the selected tenant only when the route does not explicitly name a tenant', () => {
    expect(resolveViewerTenantRoute(undefined, [wawa], wawa)).toEqual({ kind: 'DEFAULT', tenantId: wawa });
  });

  it('fails closed when an explicit tenant id is malformed instead of falling back to the selected tenant', () => {
    expect(resolveViewerTenantRoute(krabi.slice(0, -1), [wawa], wawa)).toEqual({ kind: 'UNAVAILABLE' });
  });

  it('fails closed when an explicit tenant is not authorized instead of falling back to the selected tenant', () => {
    expect(resolveViewerTenantRoute(krabi, [wawa], wawa)).toEqual({ kind: 'UNAVAILABLE' });
  });

  it('accepts an authorized explicit tenant without consulting the selected tenant', () => {
    expect(resolveViewerTenantRoute(krabi, [wawa, krabi], wawa)).toEqual({ kind: 'EXPLICIT', tenantId: krabi });
  });
});

describe('parseViewerEntryReferences', () => {
  const deliveryReference = 'd'.repeat(32);
  const invitationReference = 'i'.repeat(32);

  it('recovers an invitation reference from the LIFF callback state', () => {
    expect(parseViewerEntryReferences({
      'liff.state': `/invite?ref=${invitationReference}`,
      code: 'transient-oauth-code',
      state: 'transient-oauth-state'
    }, false)).toEqual({ invitationReference });
  });

  it('leaves a valid non-invitation LIFF state for the SDK to process', () => {
    expect(parseViewerEntryReferences({
      'liff.state': `/tenant/${krabi}`,
      code: 'transient-oauth-code',
      state: 'transient-oauth-state'
    }, false)).toEqual({});
  });

  it.each([
    { 'liff.state': `https://attacker.example/invite?ref=${invitationReference}` },
    { 'liff.state': `//attacker.example/invite?ref=${invitationReference}` },
    { 'liff.state': ['/invite?ref=one', '/invite?ref=two'] },
    { 'liff.state': `/invite?ref=${invitationReference}&ref=${invitationReference}` },
    { 'liff.state': `/invite?ref=${invitationReference}&deliveryRef=${deliveryReference}` }
  ])('rejects an unsafe or ambiguous LIFF callback state: %o', (query) => {
    expect(parseViewerEntryReferences(query, false)).toEqual({ error: 'VIEWER_LINK_INVALID' });
  });

  it('accepts one bounded delivery reference', () => {
    expect(parseViewerEntryReferences({ deliveryRef: deliveryReference }, false)).toEqual({ deliveryReference });
  });

  it.each([
    { deliveryRef: ['a', 'b'] },
    { deliveryRef: '' },
    { deliveryRef: 'short' },
    { deliveryRef: 'x'.repeat(513) }
  ])('rejects ambiguous or malformed delivery references: %o', (query) => {
    expect(parseViewerEntryReferences(query, false)).toEqual({ error: 'VIEWER_LINK_INVALID' });
  });

  it('rejects mixed invitation and delivery references', () => {
    expect(parseViewerEntryReferences({ ref: invitationReference, deliveryRef: deliveryReference }, true)).toEqual({ error: 'VIEWER_LINK_INVALID' });
  });
});

describe('deliveryContextRoute', () => {
  it('builds only clean delivery routes from trusted context identifiers', () => {
    const deliveryId = '11111111-1111-4111-8111-111111111111';
    expect(deliveryContextRoute(krabi, deliveryId)).toBe(`/app/tenant/${krabi}/delivery/${deliveryId}`);
    expect(deliveryContextRoute(krabi, deliveryId, 'sales_goods_services')).toBe(`/app/tenant/${krabi}/delivery/${deliveryId}/report/sales_goods_services`);
  });
});

describe('explicitViewerTenantId', () => {
  it('accepts only a complete tenant UUID from the route', () => {
    expect(explicitViewerTenantId(krabi)).toBe(krabi);
    expect(explicitViewerTenantId(krabi.slice(0, -1))).toBeUndefined();
    expect(explicitViewerTenantId(undefined)).toBeUndefined();
  });
});

describe('viewer navigation scope', () => {
  const permitted = ['gross_profit_by_ar_customer', 'gross_profit_by_product', 'purchase_goods_payables', 'sales_goods_services'];
  const delivered = ['sales_goods_services'];

  it('shows only reports materialized in the LINE delivery while preserving their order', () => {
    expect(viewerNavigationScope('viewer-delivery')).toBe('DELIVERY');
    expect(viewerNavigationScope('viewer-delivery-report')).toBe('DELIVERY');
    expect(resolveViewerNavigationReportKeys('DELIVERY', permitted, delivered)).toEqual(delivered);
  });

  it('shows every currently permitted report after leaving the delivery context', () => {
    expect(viewerNavigationScope('viewer-overview')).toBe('TENANT');
    expect(resolveViewerNavigationReportKeys('TENANT', permitted, delivered)).toEqual(permitted);
  });

  it('does not fall back to tenant permissions while a delivery context is unresolved', () => {
    expect(resolveViewerNavigationReportKeys('DELIVERY', permitted)).toEqual([]);
  });
});

describe('resolveViewerHome', () => {
  it('auto-opens the only authorized tenant', () => {
    expect(resolveViewerHome([wawa])).toEqual({ kind: 'AUTO', tenantId: wawa });
  });

  it('requires an explicit choice when more than one tenant is authorized', () => {
    expect(resolveViewerHome([wawa, krabi])).toEqual({ kind: 'CHOOSE' });
  });

  it('shows an empty state when no tenant is authorized', () => {
    expect(resolveViewerHome([])).toEqual({ kind: 'EMPTY' });
  });
});
