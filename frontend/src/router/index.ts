import { createRouter, createWebHistory } from 'vue-router';
import { useAdminSession } from '@/stores/session';

const router = createRouter({
  history: createWebHistory(),
  scrollBehavior: () => ({ top: 0 }),
  routes: [
    { path: '/', redirect: '/app' },
    { path: '/admin/login', name: 'admin-login', component: () => import('@/views/admin/AdminLogin.vue'), meta: { guestOnly: true } },
    {
      path: '/admin',
      component: () => import('@/layout/AppLayout.vue'),
      meta: { requiresAdmin: true },
      children: [
        { path: '', name: 'admin-dashboard', component: () => import('@/views/admin/AdminDashboard.vue'), meta: { pageTitle: 'ภาพรวมระบบ' } },
        { path: 'password', name: 'admin-password', component: () => import('@/views/admin/AdminPassword.vue'), meta: { pageTitle: 'ตั้งรหัสผ่านใหม่' } },
        { path: 'tenants', name: 'admin-tenants', component: () => import('@/views/admin/TenantList.vue'), meta: { pageTitle: 'ร้านค้า' } },
        { path: 'tenants/:tenantId/recipients/:recipientId/permissions', name: 'admin-recipient-permissions', component: () => import('@/views/admin/RecipientPermissions.vue'), meta: { pageTitle: 'กำหนดสิทธิ์รายงาน' } },
        { path: 'tenants/:tenantId/schedules/new', name: 'admin-schedule-new', component: () => import('@/views/admin/ScheduleEditor.vue'), meta: { pageTitle: 'เพิ่มตารางส่งรายงาน' } },
        { path: 'tenants/:tenantId/schedules/:scheduleId/edit', name: 'admin-schedule-edit', component: () => import('@/views/admin/ScheduleEditor.vue'), meta: { pageTitle: 'แก้ไขตารางส่งรายงาน' } },
        { path: 'tenants/:tenantId', name: 'admin-tenant-detail', component: () => import('@/views/admin/TenantDetail.vue'), meta: { pageTitle: 'รายละเอียดร้าน' } },
        { path: 'report-runs', name: 'admin-report-runs', component: () => import('@/views/admin/ReportRuns.vue'), meta: { pageTitle: 'การสร้างรายงาน' } },
        { path: 'deliveries', name: 'admin-deliveries', component: () => import('@/views/admin/Deliveries.vue'), meta: { pageTitle: 'การส่ง LINE' } },
        { path: 'audit', name: 'admin-audit', component: () => import('@/views/admin/AuditLogs.vue'), meta: { pageTitle: 'ประวัติการใช้งาน' } },
        { path: 'operational-incidents', name: 'admin-operational-incidents', component: () => import('@/views/admin/OperationalIncidents.vue'), meta: { pageTitle: 'เหตุสำคัญ' } },
        { path: 'operational-incidents/:incidentId', name: 'admin-operational-incident', component: () => import('@/views/admin/OperationalIncidentDetail.vue'), meta: { pageTitle: 'รายละเอียดเหตุสำคัญ' } }
      ]
    },
    {
      path: '/app',
      component: () => import('@/views/viewer/ViewerShell.vue'),
      children: [
        { path: '', name: 'viewer-home', component: () => import('@/views/viewer/ViewerHome.vue') },
        { path: 'invite', name: 'viewer-invite', component: () => import('@/views/viewer/ViewerHome.vue') },
        { path: 'tenant/:tenantId', name: 'viewer-overview', component: () => import('@/views/viewer/ExecutiveOverview.vue') },
        { path: 'tenant/:tenantId/delivery/:deliveryId', name: 'viewer-delivery', component: () => import('@/views/viewer/ViewerDelivery.vue') },
        { path: 'tenant/:tenantId/delivery/:deliveryId/report/:reportKey', name: 'viewer-delivery-report', component: () => import('@/views/viewer/ViewerDeliveryReport.vue') },
        { path: 'tenant/:tenantId/report/:reportKey', name: 'viewer-report', component: () => import('@/views/viewer/ViewerReport.vue') }
      ]
    },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: () => import('@/views/NotFound.vue') }
  ]
});

router.beforeEach(async (to) => {
  const { state, ensureAdminSession } = useAdminSession();
  if (to.meta.requiresAdmin) {
    let authenticated: boolean;
    try { authenticated = await ensureAdminSession(); }
    catch { return { name: 'admin-login', query: { redirect: to.fullPath, sessionError: '1' } }; }
    if (!authenticated) return { name: 'admin-login', query: { redirect: to.fullPath } };
    if (state.session?.mustRotateBootstrapPassword && to.name !== 'admin-password') return { name: 'admin-password' };
  }
  if (to.meta.guestOnly) {
    let authenticated: boolean;
    try { authenticated = await ensureAdminSession(); }
    catch { return true; }
    if (authenticated) return { name: state.session?.mustRotateBootstrapPassword ? 'admin-password' : 'admin-dashboard' };
  }
  return true;
});

export default router;
