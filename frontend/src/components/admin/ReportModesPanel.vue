<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useToast } from 'primevue/usetoast';
import { adminApi, ApiError, type ReportMode, type ReportModeItem, type ReportModeMeasurement } from '@/api';
import { errorMessage, formatDateTime } from '@/utils/format';
import { formatModeDuration, formatModeRows, measurementSummary, modeLabel, modeSeverity, sourceLabel } from '@/utils/executionModePresentation';

const props = defineProps<{ tenantId: string }>();
const toast = useToast();

const items = ref<ReportModeItem[]>([]);
const loading = ref(true);
const loadError = ref('');
const savingKey = ref('');
const measuring = ref(false);
const measurements = ref<ReportModeMeasurement[]>([]);
const measureNotice = ref<{ severity: 'warn' | 'error'; text: string } | null>(null);

const labelFor = computed(() => new Map(items.value.map((item) => [item.reportKey, item.label])));
const measurementLines = computed(() => measurements.value.map((item) => measurementSummary(item, labelFor.value.get(item.reportKey) ?? item.reportKey)));

async function load() {
  loading.value = true;
  loadError.value = '';
  try {
    items.value = (await adminApi.listReportModes(props.tenantId)).data;
  } catch (error) {
    loadError.value = errorMessage(error);
  } finally {
    loading.value = false;
  }
}

async function change(item: ReportModeItem) {
  const next: ReportMode = item.mode === 'CHUNKED' ? 'DIRECT' : 'CHUNKED';
  savingKey.value = item.reportKey;
  try {
    const updated = await adminApi.setReportMode(props.tenantId, item.reportKey, next);
    items.value = items.value.map((row) => row.reportKey === updated.reportKey ? updated : row);
    toast.add({ severity: 'success', summary: `ตั้ง${item.label}เป็น${modeLabel(next)}แล้ว`, life: 3000 });
  } catch (error) {
    toast.add({ severity: 'error', summary: 'เปลี่ยนโหมดไม่สำเร็จ', detail: errorMessage(error), life: 5000 });
  } finally {
    savingKey.value = '';
  }
}

async function measure() {
  measuring.value = true;
  measureNotice.value = null;
  try {
    measurements.value = (await adminApi.measureReportModes(props.tenantId)).data;
    await load();
  } catch (error) {
    measurements.value = [];
    if (error instanceof ApiError && error.code === 'TENANT_BUSY') {
      measureNotice.value = { severity: 'warn', text: 'ร้านนี้กำลังสร้างรายงานอยู่ ให้รอจนเสร็จแล้วกดวัดขนาดอีกครั้ง' };
    } else if (error instanceof ApiError && error.code === 'SML_NOT_CONFIGURED') {
      measureNotice.value = { severity: 'warn', text: 'ร้านนี้ยังไม่ได้ตั้งค่าการเชื่อมต่อ SML' };
    } else {
      measureNotice.value = { severity: 'error', text: errorMessage(error) };
    }
  } finally {
    measuring.value = false;
  }
}

onMounted(load);
</script>

<template>
  <div class="grid gap-4">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="max-w-3xl">
        <h3 class="text-lg font-semibold m-0">โหมดดึงรายงาน</h3>
        <p class="m-0 mt-1 text-muted-color">
          <strong>ดึงตรง</strong> ดึงครั้งเดียว ใช้ได้กับร้านส่วนใหญ่ · <strong>แบ่งชุด</strong> ดึงทีละชุดสำหรับข้อมูลจำนวนมาก ใช้เวลานานกว่า (การ์ดอาจช้าได้ถึงราว 10 นาที) ระบบสลับเป็นแบ่งชุดเองเมื่อข้อมูลใหญ่เกินกว่าจะดึงครั้งเดียว และบันทึกไว้ในประวัติการใช้งาน
        </p>
      </div>
      <Button label="วัดขนาดร้านนี้" icon="pi pi-gauge" :loading="measuring" :disabled="measuring || loading" @click="measure" />
    </div>

    <Message v-if="measureNotice" :severity="measureNotice.severity" :closable="false">{{ measureNotice.text }}</Message>
    <Message v-if="measurementLines.length" severity="info" :closable="false">
      <div class="grid gap-1" data-testid="measure-result"><div v-for="line in measurementLines" :key="line">{{ line }}</div></div>
    </Message>

    <Message v-if="loadError" severity="error" :closable="false">
      <div class="flex flex-wrap items-center gap-3">{{ loadError }} <Button label="ลองใหม่" size="small" text @click="load" /></div>
    </Message>
    <DataTable v-else :value="items" :loading="loading" data-key="reportKey" size="small" striped-rows responsive-layout="scroll">
      <Column header="รายงาน"><template #body="{ data }"><span class="font-medium">{{ data.label }}</span></template></Column>
      <Column header="โหมด"><template #body="{ data }"><Tag :severity="modeSeverity(data.mode)" :value="modeLabel(data.mode)" /></template></Column>
      <Column header="ที่มา"><template #body="{ data }">
        <div>{{ sourceLabel(data.source) }}</div>
        <small v-if="data.reason" class="text-muted-color">{{ data.reason }}</small>
        <small v-if="data.changedAt && data.source !== 'DEFAULT'" class="text-muted-color block">{{ formatDateTime(data.changedAt) }}</small>
      </template></Column>
      <Column header="ขนาดที่วัดล่าสุด"><template #body="{ data }"><span class="tabular">{{ formatModeRows(data.lastRows) }}</span></template></Column>
      <Column header="เวลาที่ใช้"><template #body="{ data }"><span class="tabular">{{ formatModeDuration(data.lastDurationMs) }}</span></template></Column>
      <Column header="">
        <template #body="{ data }">
          <Button v-if="data.chunkable" :label="data.mode === 'CHUNKED' ? 'เปลี่ยนเป็นดึงตรง' : 'เปลี่ยนเป็นแบ่งชุด'" size="small" outlined :loading="savingKey === data.reportKey" :disabled="savingKey !== ''" @click="change(data)" />
          <small v-else class="text-muted-color">แบ่งชุดยังไม่ได้</small>
        </template>
      </Column>
      <template #empty><div class="py-6 text-center text-muted-color">ไม่พบรายงาน</div></template>
    </DataTable>
  </div>
</template>

<style scoped>
.tabular { font-variant-numeric: tabular-nums; }
</style>
