<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import type { DataTableFilterEvent } from 'primevue/datatable';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';
import { useConfirm } from 'primevue/useconfirm';
import { ApiError, adminApi, type Tenant, type TenantInput } from '@/api';
import { newIdempotencyKey } from '@/api/client';
import { errorMessage, formatDate } from '@/utils/format';
import { statusLabel as operationalStatusLabel } from '@/utils/status';
import SakaiTableHeader from '@/components/table/SakaiTableHeader.vue';
import { useServerTable } from '@/composables/useServerTable';
import { useSakaiFilterMenu } from '@/composables/useSakaiFilterMenu';

const router = useRouter();
const toast = useToast();
const confirm = useConfirm();
const createOpen = ref(false);
const saving = ref(false);
const archivingTenantId = ref('');
const createError = ref('');
let createActionKey = '';

function bangkokCalendarDate() {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone: 'Asia/Bangkok', year: 'numeric', month: '2-digit', day: '2-digit'
  }).formatToParts(new Date());
  const value = Object.fromEntries(parts.map((part) => [part.type, part.value]));
  return new Date(Number(value.year), Number(value.month) - 1, Number(value.day));
}

function defaultAccessEndDate() {
  const date = bangkokCalendarDate();
  date.setFullYear(date.getFullYear() + 1);
  return date;
}

function bangkokEndOfDayISO(date: Date) {
  const year = String(date.getFullYear()).padStart(4, '0');
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return new Date(`${year}-${month}-${day}T23:59:59.999+07:00`).toISOString();
}

const form = reactive<{ name: string; accessEndsAt: Date | null }>({ name: '', accessEndsAt: defaultAccessEndDate() });
const statusOptions = [{ label: 'ใช้งาน', value: 'ACTIVE' }, { label: 'ปิดใช้งาน', value: 'DISABLED' }, { label: 'หมดอายุ', value: 'EXPIRED' }];
const readinessOptions = [{ label: 'พร้อมใช้งาน', value: 'READY' }, { label: 'เชื่อมต่อไม่สำเร็จ', value: 'FAILED' }, { label: 'ยังไม่ตั้งค่า', value: 'UNCONFIGURED' }];
const primeFilters = ref({
  status: { value: null as string[] | null, matchMode: 'in' },
  smlReadiness: { value: null as string[] | null, matchMode: 'in' }
});
useSakaiFilterMenu(primeFilters);
type SMLReadiness = 'READY' | 'FAILED' | 'UNCONFIGURED';
const table = useServerTable<Tenant, { statuses: Tenant['status'][]; smlReadiness: SMLReadiness[] }>({
  initialFilters: { statuses: [], smlReadiness: [] },
  query: (input, signal) => adminApi.queryTenants(input, signal)
});
const tenants = table.rows;
const loading = table.loading;
const error = table.error;
const hasFilters = computed(() => Boolean(table.appliedGlobalSearch.value || table.appliedFilters.value.statuses.length || table.appliedFilters.value.smlReadiness.length));

function applyPrimeFilters(event: DataTableFilterEvent) {
  table.draftFilters.value.statuses = ((event.filters.status as { value?: string[] | null })?.value ?? []) as Tenant['status'][];
  table.draftFilters.value.smlReadiness = ((event.filters.smlReadiness as { value?: string[] | null })?.value ?? []) as SMLReadiness[];
  void table.applyFilters();
}

function clearTableFilters() {
  primeFilters.value.status.value = null;
  primeFilters.value.smlReadiness.value = null;
  void table.clearFilters();
}

async function createTenant() {
  if (saving.value) return;
  createError.value = '';
  if (!form.name.trim()) {
    createError.value = 'กรุณาระบุชื่อร้าน';
    return;
  }
  if (!form.accessEndsAt) {
    createError.value = 'กรุณาระบุวันสิ้นสุดสิทธิ์';
    return;
  }
  if (form.accessEndsAt < bangkokCalendarDate()) {
    createError.value = 'วันสิ้นสุดสิทธิ์ต้องไม่เป็นวันที่ผ่านมาแล้ว';
    return;
  }
  saving.value = true;
  try {
    const input: TenantInput = { name: form.name.trim(), accessEndsAt: bangkokEndOfDayISO(form.accessEndsAt) };
    createActionKey ||= newIdempotencyKey('tenant');
    const created = await adminApi.createTenant(input, createActionKey);
    createActionKey = '';
    createOpen.value = false;
    toast.add({ severity: 'success', summary: 'สร้างร้านค้าแล้ว', detail: created.name, life: 3000 });
    await router.push(`/admin/tenants/${created.id}`);
  } catch (cause) { if (!(cause instanceof ApiError) || !cause.retryable) createActionKey = ''; createError.value = errorMessage(cause); }
  finally { saving.value = false; }
}

function openCreateDialog() {
  Object.assign(form, { name: '', accessEndsAt: defaultAccessEndDate() });
  createError.value = '';
  createActionKey = '';
  createOpen.value = true;
}

async function archiveTenant(item: Tenant) {
  if (archivingTenantId.value) return;
  archivingTenantId.value = item.id;
  try {
    await adminApi.archiveTenant(item.id, item.version);
    await table.refresh();
    toast.add({
      severity: 'success',
      summary: 'ลบร้านค้าแล้ว',
      detail: 'หยุดตารางส่งรายงานและเพิกถอนสิทธิ์ของร้านแล้ว โดยยังเก็บประวัติรายงาน การส่ง LINE และ Audit ไว้',
      life: 5000
    });
  } catch (cause) {
    if (cause instanceof ApiError && cause.code === 'VERSION_CONFLICT') {
      toast.add({ severity: 'warn', summary: 'ข้อมูลร้านมีการเปลี่ยนแปลง', detail: 'ระบบกำลังโหลดข้อมูลล่าสุด กรุณาตรวจสอบแล้วลองลบอีกครั้ง', life: 5000 });
      await table.refresh();
    } else {
      toast.add({ severity: 'error', summary: 'ลบร้านค้าไม่สำเร็จ', detail: errorMessage(cause), life: 6000 });
    }
  } finally {
    if (archivingTenantId.value === item.id) archivingTenantId.value = '';
  }
}

function confirmArchiveTenant(item: Tenant) {
  if (archivingTenantId.value) return;
  confirm.require({
    header: 'ยืนยันลบร้านค้า',
    message: `ลบร้าน “${item.name}”? ระบบจะหยุดตารางส่งรายงาน ยกเลิกลิงก์เชิญ และเพิกถอนสิทธิ์ Dashboard/LINE ของร้านทันที ร้านจะหายจากรายการและกู้คืนผ่านหน้าจอไม่ได้ แต่ประวัติรายงาน การส่ง LINE และ Audit เดิมยังคงอยู่ตามระยะเวลาเก็บข้อมูล`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'ลบร้านค้า',
    rejectLabel: 'ยกเลิก',
    acceptClass: 'p-button-danger',
    accept: () => archiveTenant(item)
  });
}

function statusSeverity(value: Tenant['status']) { return value === 'ACTIVE' ? 'success' : value === 'EXPIRED' ? 'danger' : 'secondary'; }
function statusLabel(value: Tenant['status']) { return value === 'ACTIVE' ? 'ใช้งาน' : value === 'EXPIRED' ? 'หมดอายุ' : 'ปิดใช้งาน'; }
watch(form, () => { if (!saving.value) createActionKey = ''; }, { deep: true });
</script>

<template>
  <AppPageHeader title="ร้านค้า" subtitle="สิทธิ์การใช้งานและความพร้อมของ SML" />
  <div class="card table-card">
    <Message v-if="error" severity="error" :closable="false" class="mb-4">โหลดข้อมูลใหม่ไม่สำเร็จ ข้อมูลเดิมยังแสดงอยู่ · {{ error }} <Button label="ลองใหม่" text size="small" @click="table.refresh()" /></Message>
    <DataTable v-model:filters="primeFilters" :value="tenants" :loading="loading" data-key="id" lazy paginator :first="table.page.value * table.pageSize.value" :rows="table.pageSize.value" :total-records="table.total.value" :rows-per-page-options="[25, 50, 100]" filter-display="menu" row-hover show-gridlines scrollable current-page-report-template="หน้า {currentPage} จาก {totalPages} · ทั้งหมด {totalRecords} รายการ" paginator-template="RowsPerPageDropdown FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink CurrentPageReport" @page="table.changePage" @filter="applyPrimeFilters">
      <template #header><SakaiTableHeader v-model:global-search="table.globalSearch.value" :loading="loading" :has-filters="hasFilters" @clear="clearTableFilters"><template #start><Button label="เพิ่มร้านค้า" icon="pi pi-plus" @click="openCreateDialog" /></template></SakaiTableHeader></template>
      <Column field="name" header="ร้านค้า" frozen><template #body="{ data }"><button class="text-left bg-transparent border-0 p-0 text-primary font-semibold cursor-pointer" @click="router.push(`/admin/tenants/${data.id}`)">{{ data.name }}</button></template></Column>
      <Column field="status" header="สถานะ" :show-filter-match-modes="false"><template #body="{ data }"><Tag :severity="statusSeverity(data.status)" :value="statusLabel(data.status)" /></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="statusOptions" option-label="label" option-value="value" placeholder="ทุกสถานะ" /></template></Column>
      <Column field="smlReadiness" header="SML" :show-filter-match-modes="false"><template #body="{ data }"><Tag :severity="data.smlReadiness === 'READY' ? 'success' : data.smlReadiness === 'FAILED' ? 'danger' : 'warn'" :value="operationalStatusLabel(data.smlReadiness || 'UNCONFIGURED')" /></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="readinessOptions" option-label="label" option-value="value" placeholder="ทุกสถานะ SML" /></template></Column>
      <Column field="accessEndsAt" header="สิ้นสุดสิทธิ์"><template #body="{ data }">{{ formatDate(data.accessEndsAt) }}</template></Column>
      <Column header="จัดการ" style="width: 8rem" header-class="table-action-column" body-class="table-action-column"><template #body="{ data }"><div class="flex items-center justify-end gap-1"><Button icon="pi pi-chevron-right" text rounded class="touch-action" :aria-label="`เปิดร้าน ${data.name}`" v-tooltip.top="'เปิดรายละเอียดร้าน'" @click="router.push(`/admin/tenants/${data.id}`)" /><Button icon="pi pi-trash" severity="danger" text rounded class="touch-action" :aria-label="`ลบร้าน ${data.name}`" v-tooltip.top="'ลบร้านค้า'" :loading="archivingTenantId === data.id" :disabled="Boolean(archivingTenantId)" @click="confirmArchiveTenant(data)" /></div></template></Column>
      <template #empty><div class="py-8 text-center text-muted-color">ไม่พบร้านค้าที่ตรงกับเงื่อนไข <Button v-if="hasFilters" label="ล้างตัวกรอง" text size="small" @click="clearTableFilters" /></div></template>
    </DataTable>
  </div>

  <Dialog v-model:visible="createOpen" modal header="เพิ่มร้านค้า" class="responsive-dialog" :style="{ width: '34rem' }">
    <Message v-if="createError" severity="error" :closable="false" class="mb-4">{{ createError }}</Message>
    <form id="create-tenant" class="grid gap-4" @submit.prevent="createTenant">
      <div class="grid gap-2"><label for="tenant-name">ชื่อร้าน</label><InputText id="tenant-name" v-model="form.name" maxlength="160" fluid /></div>
      <div class="grid gap-2">
        <label for="tenant-access-end">สิ้นสุดสิทธิ์</label>
        <DatePicker input-id="tenant-access-end" v-model="form.accessEndsAt" date-format="dd/mm/yy" :min-date="bangkokCalendarDate()" show-icon fluid />
        <small class="text-muted-color">สิทธิ์จะสิ้นสุดเวลา 23:59 น. ตามเวลาไทยของวันที่เลือก</small>
      </div>
      <Message severity="info" :closable="false">ระบบจะสร้างรหัสภายในและตั้งเขตเวลาไทยให้อัตโนมัติ</Message>
    </form>
    <template #footer><Button label="ยกเลิก" text :disabled="saving" @click="createOpen = false" /><Button type="submit" form="create-tenant" label="สร้างร้านค้า" icon="pi pi-check" :loading="saving" :disabled="saving" /></template>
  </Dialog>
</template>
