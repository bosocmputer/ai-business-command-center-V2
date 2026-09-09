<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';
import { adminApi, ApiError, type AdminReportCatalog, type PermissionDependencies, type Recipient, type ReportKey, type Tenant } from '@/api';
import ReportPickerPanel from '@/components/admin/ReportPickerPanel.vue';
import { beginAdminTenantContext, setAdminTenantContext } from '@/stores/adminTenantContext';
import { loadAdminReportCatalog } from '@/stores/reportCatalog';
import { errorMessage } from '@/utils/format';

const route = useRoute();
const router = useRouter();
const toast = useToast();
const tenantId = String(route.params.tenantId);
beginAdminTenantContext(tenantId);
const recipientId = String(route.params.recipientId);
const tenant = ref<Tenant>();
const recipient = ref<Recipient>();
const catalog = ref<AdminReportCatalog>();
const selected = ref<ReportKey[]>([]);
const baseline = ref<ReportKey[]>([]);
const loading = ref(true);
const saving = ref(false);
const error = ref('');
const conflict = ref('');
const dependencySchedules = ref<string[]>([]);
const dependencies = ref<PermissionDependencies>();
const dependencyLoading = ref(false);
const dependencyError = ref('');
const controller = new AbortController();

const selectedSet = computed(() => new Set(selected.value));
const baselineSet = computed(() => new Set(baseline.value));
const added = computed(() => selected.value.filter((key) => !baselineSet.value.has(key)));
const removed = computed(() => baseline.value.filter((key) => !selectedSet.value.has(key)));
const dirty = computed(() => added.value.length > 0 || removed.value.length > 0);
const lockedKeys = computed(() => dependencies.value?.items.map((item) => item.reportKey) ?? []);
const lockedSet = computed(() => new Set(lockedKeys.value));
const unsafeRemoval = computed(() => removed.value.some((key) => lockedSet.value.has(key)) || (!!dependencyError.value && removed.value.length > 0));

async function loadDependencies(signal?: AbortSignal) {
  dependencyLoading.value = true;
  dependencyError.value = '';
  try { dependencies.value = await adminApi.permissionDependencies(tenantId, recipientId, signal); }
  catch (cause) { dependencies.value = undefined; dependencyError.value = errorMessage(cause); }
  finally { dependencyLoading.value = false; }
}

async function load() {
  loading.value = true;
  error.value = '';
  try {
    const [tenantResult, recipientResult, catalogResult] = await Promise.all([
      adminApi.getTenant(tenantId), adminApi.getRecipient(tenantId, recipientId, controller.signal), loadAdminReportCatalog(controller.signal)
    ]);
    tenant.value = tenantResult;
    setAdminTenantContext(tenantId, tenantResult.name);
    recipient.value = recipientResult;
    catalog.value = catalogResult;
    selected.value = [...recipientResult.reportKeys];
    baseline.value = [...recipientResult.reportKeys];
    await loadDependencies(controller.signal);
  } catch (cause) { error.value = errorMessage(cause); }
  finally { loading.value = false; }
}

function sameKeys(left: ReportKey[], right: ReportKey[]) {
  if (left.length !== right.length) return false;
  const expected = new Set(left);
  return right.every((key) => expected.has(key));
}

async function save() {
  if (!recipient.value || saving.value || !dirty.value || unsafeRemoval.value) return;
  saving.value = true;
  conflict.value = '';
  dependencySchedules.value = [];
  const desired = [...selected.value];
  try {
    const updated = await adminApi.replacePermissions(tenantId, recipientId, desired, recipient.value.permissionsVersion);
    recipient.value = updated;
    baseline.value = [...updated.reportKeys];
    selected.value = [...updated.reportKeys];
    toast.add({ severity: 'success', summary: 'บันทึกสิทธิ์แล้ว', detail: `${updated.reportKeys.length} รายงาน`, life: 3000 });
  } catch (cause) {
    if (cause instanceof ApiError && (cause.code === 'VERSION_CONFLICT' || cause.code === 'TIMEOUT')) {
      try {
        const latest = await adminApi.getRecipient(tenantId, recipientId);
        recipient.value = latest;
        if (sameKeys(desired, latest.reportKeys)) {
          baseline.value = [...latest.reportKeys]; selected.value = [...latest.reportKeys];
          toast.add({ severity: 'success', summary: 'บันทึกสิทธิ์แล้ว', life: 3000 });
        } else {
          baseline.value = [...latest.reportKeys];
          conflict.value = cause.code === 'VERSION_CONFLICT'
            ? 'สิทธิ์ถูกแก้จากอีกหน้าจอแล้ว ระบบโหลดค่าล่าสุดมาเปรียบเทียบ กรุณาตรวจสอบรายการก่อนบันทึกอีกครั้ง'
            : 'ไม่สามารถยืนยันผลการบันทึกได้ ระบบโหลดค่าล่าสุดแล้ว กรุณาตรวจสอบก่อนลองใหม่';
        }
      } catch (refreshCause) { conflict.value = `ตรวจสอบผลการบันทึกไม่ได้: ${errorMessage(refreshCause)}`; }
    } else if (cause instanceof ApiError && cause.code === 'PERMISSION_IN_USE') {
      dependencySchedules.value = (cause.fieldErrors ?? []).map((item) => item.message);
      await loadDependencies();
    } else {
      toast.add({ severity: 'error', summary: 'บันทึกสิทธิ์ไม่สำเร็จ', detail: errorMessage(cause), life: 6000 });
    }
  } finally { saving.value = false; }
}

function back() { void router.push({ name: 'admin-tenant-detail', params: { tenantId }, query: { tab: 'recipients' } }); }
function beforeUnload(event: BeforeUnloadEvent) { if (dirty.value) { event.preventDefault(); event.returnValue = ''; } }
onBeforeRouteLeave(() => !dirty.value || window.confirm('มีสิทธิ์ที่ยังไม่ได้บันทึก ต้องการออกจากหน้านี้หรือไม่'));
onMounted(() => { window.addEventListener('beforeunload', beforeUnload); void load(); });
onBeforeUnmount(() => { controller.abort('unmount'); window.removeEventListener('beforeunload', beforeUnload); });
</script>

<template>
  <div v-if="loading" class="card"><Skeleton height="2rem" width="18rem" class="mb-4" /><Skeleton height="24rem" /></div>
  <Message v-else-if="error" severity="error" :closable="false">{{ error }} <Button label="ลองใหม่" text @click="load" /></Message>
  <template v-else-if="tenant && recipient && catalog">
    <AppPageHeader title="กำหนดสิทธิ์รายงาน" :subtitle="`${recipient.displayName} · ร้าน ${tenant.name}`" mobile-mode="entity"><template #back><Button label="ผู้รับและสิทธิ์" icon="pi pi-arrow-left" text class="entity-back-action -ml-3 mb-1 touch-action" @click="back" /></template><template #actions><Tag :severity="recipient.status === 'ACTIVE' ? 'success' : 'warn'" :value="recipient.status === 'ACTIVE' ? 'ยืนยัน LINE แล้ว' : 'รอยืนยัน LINE'" /></template></AppPageHeader>
    <Message v-if="conflict" severity="warn" :closable="false" class="mb-4">{{ conflict }}</Message>
    <Message v-if="dependencySchedules.length" severity="error" :closable="false" class="mb-4">สิทธิ์ที่นำออกยังถูกใช้โดยตารางส่ง: {{ dependencySchedules.join(', ') }} กรุณาพักหรือแก้ตารางส่งก่อน</Message>
    <Message v-if="dependencyError" severity="warn" :closable="false" class="mb-4">ตรวจสอบตาราง Active ไม่สำเร็จ จึงเพิ่มสิทธิ์ได้แต่ยังไม่อนุญาตให้นำสิทธิ์เดิมออก <Button label="ตรวจสอบอีกครั้ง" text :loading="dependencyLoading" @click="loadDependencies()" /></Message>
    <div class="card">
      <Toolbar class="mb-5 border-0 p-0"><template #start><div><h2 class="text-lg font-semibold m-0">สิทธิ์เปิดดู Dashboard: {{ selected.length.toLocaleString('th-TH') }} รายงาน</h2><p class="text-muted-color mt-1 mb-0">สิทธิ์เป็นเพดานการเข้าถึง ไม่ได้ทำให้ทุกรายงานถูกส่งอัตโนมัติ รายงานใน LINE เลือกแยกในตารางส่ง LINE</p></div></template><template #end><div class="flex flex-wrap items-center gap-3"><span v-if="dirty" class="text-orange-600 text-sm">เพิ่ม {{ added.length }} · นำออก {{ removed.length }}</span><Button label="บันทึกสิทธิ์" icon="pi pi-save" class="touch-action" :loading="saving" :disabled="saving || !dirty || unsafeRemoval || dependencyLoading" @click="save" /></div></template></Toolbar>
      <Message v-if="lockedKeys.length" severity="info" :closable="false" class="mb-4">มี {{ lockedKeys.length }} สิทธิ์ที่ยังนำออกไม่ได้ เพราะถูกใช้ในตารางส่ง LINE ที่กำลังใช้งาน กรุณาพักหรือแก้ตารางก่อน</Message>
      <ReportPickerPanel v-model="selected" :definitions="catalog.data" :locked-keys="lockedKeys" />
      <div v-if="dependencies?.items.length" class="mt-5 grid gap-2"><div class="font-semibold">ตาราง Active ที่ใช้สิทธิ์นี้</div><div v-for="item in dependencies.items" :key="item.reportKey" class="text-sm text-muted-color"><strong>{{ catalog.data.find((definition) => definition.reportKey === item.reportKey)?.label ?? item.reportKey }}</strong>: {{ item.schedules.map((schedule) => schedule.name).join(', ') }}<span v-if="item.additionalCount"> และอีก {{ item.additionalCount }} ตาราง</span></div></div>
    </div>
  </template>
</template>
