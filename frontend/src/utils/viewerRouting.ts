const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

export type ViewerTenantRouteResolution =
  | { kind: 'DEFAULT'; tenantId: string }
  | { kind: 'EXPLICIT'; tenantId: string }
  | { kind: 'UNAVAILABLE' };

export type ViewerNavigationScope = 'TENANT' | 'DELIVERY';

export type ViewerHomeResolution =
  | { kind: 'EMPTY' }
  | { kind: 'AUTO'; tenantId: string }
  | { kind: 'CHOOSE' };

export function viewerNavigationScope(routeName: unknown): ViewerNavigationScope {
  return routeName === 'viewer-delivery' || routeName === 'viewer-delivery-report' ? 'DELIVERY' : 'TENANT';
}

export function resolveViewerNavigationReportKeys<T extends string>(
  scope: ViewerNavigationScope,
  permittedReportKeys: T[],
  deliveredReportKeys?: T[]
): T[] {
  return scope === 'DELIVERY' ? [...(deliveredReportKeys ?? [])] : [...permittedReportKeys];
}

export function resolveViewerHome(authorizedTenantIds: string[]): ViewerHomeResolution {
  if (authorizedTenantIds.length === 0) return { kind: 'EMPTY' };
  if (authorizedTenantIds.length === 1) return { kind: 'AUTO', tenantId: authorizedTenantIds[0]! };
  return { kind: 'CHOOSE' };
}

export function resolveViewerTenantRoute(
  routeTenantId: unknown,
  authorizedTenantIds: string[],
  selectedTenantId: string
): ViewerTenantRouteResolution {
  if (routeTenantId !== undefined) {
    if (typeof routeTenantId !== 'string' || !uuidPattern.test(routeTenantId) || !authorizedTenantIds.includes(routeTenantId)) {
      return { kind: 'UNAVAILABLE' };
    }
    return { kind: 'EXPLICIT', tenantId: routeTenantId };
  }

  const tenantId = authorizedTenantIds.includes(selectedTenantId) ? selectedTenantId : authorizedTenantIds[0] ?? '';
  return tenantId ? { kind: 'DEFAULT', tenantId } : { kind: 'UNAVAILABLE' };
}

export type ViewerEntryReferences = {
  invitationReference?: string;
  deliveryReference?: string;
  error?: 'VIEWER_LINK_INVALID';
};

export function parseViewerEntryReferences(query: Record<string, unknown>, invitationRoute: boolean): ViewerEntryReferences {
  const explicitInvitationReference = invitationRoute ? boundedReference(query.ref, 32, 128) : undefined;
  const liffState = parseLiffInvitationState(query['liff.state']);
  if (liffState.error) return { error: 'VIEWER_LINK_INVALID' };
  const invitationReference = explicitInvitationReference ?? liffState.invitationReference;
  const deliveryReference = boundedReference(query.deliveryRef, 32, 512);
  const invitationPresent = query.ref !== undefined;
  const deliveryPresent = query.deliveryRef !== undefined;
  const explicitInvitationInvalid = invitationRoute && invitationPresent && !explicitInvitationReference;
  const invitationAmbiguous = Boolean(explicitInvitationReference && liffState.invitationReference);
  const deliveryInvalid = deliveryPresent && !deliveryReference;
  const referencesMixed = Boolean(invitationReference && deliveryPresent);

  if (explicitInvitationInvalid || invitationAmbiguous || deliveryInvalid || referencesMixed) {
    return { error: 'VIEWER_LINK_INVALID' };
  }
  return {
    ...(invitationReference ? { invitationReference } : {}),
    ...(deliveryReference ? { deliveryReference } : {})
  };
}

function parseLiffInvitationState(value: unknown): ViewerEntryReferences {
  if (value === undefined) return {};
  if (typeof value !== 'string' || value.length === 0 || value.length > 1024 || !value.startsWith('/') || value.startsWith('//')) {
    return { error: 'VIEWER_LINK_INVALID' };
  }
  let stateURL: URL;
  try { stateURL = new URL(value, 'https://liff-state.invalid'); }
  catch { return { error: 'VIEWER_LINK_INVALID' }; }
  if (stateURL.origin !== 'https://liff-state.invalid' || stateURL.hash) return { error: 'VIEWER_LINK_INVALID' };
  if (stateURL.pathname !== '/invite' && stateURL.pathname !== '/app/invite') return {};

  const keys = [...stateURL.searchParams.keys()];
  const references = stateURL.searchParams.getAll('ref');
  const invitationReference = references.length === 1 ? boundedReference(references[0], 32, 128) : undefined;
  if (keys.some((key) => key !== 'ref') || !invitationReference) return { error: 'VIEWER_LINK_INVALID' };
  return { invitationReference };
}

export function deliveryContextRoute(tenantId: string, deliveryId: string, reportKey?: string): string {
  const base = `/app/tenant/${tenantId}/delivery/${deliveryId}`;
  return reportKey ? `${base}/report/${reportKey}` : base;
}

export function explicitViewerTenantId(value: unknown): string | undefined {
  return typeof value === 'string' && uuidPattern.test(value) ? value : undefined;
}

function boundedReference(value: unknown, minimum: number, maximum: number): string | undefined {
  return typeof value === 'string' && value.length >= minimum && value.length <= maximum ? value : undefined;
}
