<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useToast } from 'primevue/usetoast';
import { adminApi, ApiError, type TenantEngagement } from '@/api';
import { errorMessage } from '@/utils/format';

const props = defineProps<{ tenantId: string }>();
const toast = useToast();

const saved = ref<TenantEngagement>();
const loading = ref(true);
const loadError = ref('');
const saving = ref(false);
const form = reactive<TenantEngagement>({
  alertChannel: 'TELEGRAM', digestEnabled: true, weeklySummaryEnabled: false, weeklySummaryWeekday: 1, weeklySummaryHour: 8,
  inactivityNoticeEnabled: true, inactivityQuietDays: 5, version: 0
});

const channels = [
  { value: 'TELEGRAM', label: 'Telegram ของเลขา AI' },
  { value: 'LINE', label: 'ข้อความ LINE' },
  { value: 'OFF', label: 'ไม่ส่งแจ้งเตือน' }
];
const weekdays = ['จันทร์', 'อังคาร', 'พุธ', 'พฤหัสบดี', 'ศุกร์', 'เสาร์', 'อาทิตย์'].map((label, index) => ({ value: index + 1, label }));
const hours = Array.from({ length: 24 }, (_, hour) => ({ value: hour, label: `${String(hour).padStart(2, '0')}:00 น.` }));

const dirty = computed(() => Boolean(saved.value) && JSON.stringify({ ...form }) !== JSON.stringify(saved.value));
const neverSaved = computed(() => saved.value?.version === 0);

function adopt(next: TenantEngagement) {
  saved.value = next;
  Object.assign(form, next);
}

async function load() {
  loading.value = true;
  loadError.value = '';
  try {
    adopt(await adminApi.getTenantEngagement(props.tenantId));
  } catch (error) {
    loadError.value = errorMessage(error);
  } finally {
    loading.value = false;
  }
}

async function save() {
  if (!saved.value || saving.value || !dirty.value) return;
  saving.value = true;
  try {
    adopt(await adminApi.updateTenantEngagement(props.tenantId, { ...form }));
    toast.add({ severity: 'success', summary: 'บันทึกแล้ว', detail: 'การดูแลเจ้าของร้านจะใช้ค่าใหม่ในรอบถัดไป', life: 4000 });
  } catch (error) {
    toast.add({ severity: 'error', summary: 'บันทึกไม่สำเร็จ', detail: error instanceof ApiError && error.code === 'VERSION_CONFLICT' ? 'ค่าถูกแก้จากอีกหน้าจอ โหลดค่าล่าสุดมาให้แล้ว' : errorMessage(error), life: 6000 });
    if (error instanceof ApiError && error.code === 'VERSION_CONFLICT') await load();
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <section class="mt-8 max-w-4xl" aria-labelledby="engagement-title">
    <h3 id="engagement-title" class="text-lg font-semibold mb-1">การดูแลเจ้าของร้าน</h3>
    <p class="text-sm text-muted-color mb-4">กำหนดว่าเลขา AI ติดต่อเจ้าของร้านนี้อย่างไร และเมื่อไรผู้ดูแลระบบจะได้รับแจ้งว่าเจ้าของเงียบไป</p>
    <Message v-if="loadError" severity="error" :closable="false" class="mb-4">{{ loadError }} <Button label="ลองใหม่" text size="small" @click="load" /></Message>
    <Message v-else-if="neverSaved" severity="secondary" :closable="false" class="mb-4">ร้านนี้ยังไม่เคยตั้งค่า ใช้ค่าเริ่มต้นด้านล่างอยู่</Message>
    <form v-if="!loadError && saved" class="grid grid-cols-1 md:grid-cols-2 gap-5" @submit.prevent="save">
      <div class="grid gap-2">
        <label for="eng-channel">ช่องทางส่งแจ้งเตือนของเลขา AI</label>
        <Select input-id="eng-channel" v-model="form.alertChannel" :options="channels" option-label="label" option-value="value" fluid />
        <small class="text-muted-color">แจ้งเตือนทำงานโดยไม่เสียค่าโมเดล เลือก LINE เมื่อเจ้าของใช้ LINE เป็นหลักและไม่ได้เปิด Telegram</small>
      </div>
      <div class="grid gap-2 content-start">
        <label for="eng-digest">สรุปเช้าของเลขา AI</label>
        <div class="flex items-center gap-3"><ToggleSwitch input-id="eng-digest" v-model="form.digestEnabled" :disabled="form.alertChannel === 'OFF'" /><span>{{ form.digestEnabled && form.alertChannel !== 'OFF' ? 'ส่งทุกเช้า' : 'ไม่ส่ง' }}</span></div>
        <small class="text-muted-color">ร้านที่ได้การ์ดรายงานทาง LINE อยู่แล้วอาจปิดข้อนี้เพื่อไม่ให้ข้อความซ้ำกัน</small>
      </div>
      <div class="grid gap-2 content-start">
        <label for="eng-weekly">สรุปประจำสัปดาห์ (ข้อความ LINE สั้น ๆ)</label>
        <div class="flex items-center gap-3"><ToggleSwitch input-id="eng-weekly" v-model="form.weeklySummaryEnabled" /><span>{{ form.weeklySummaryEnabled ? 'เปิด' : 'ปิด' }}</span></div>
        <small class="text-muted-color">นับโควตาข้อความ LINE ของระบบ สัปดาห์ที่ไม่มีอะไรให้บอกจะไม่ส่ง</small>
      </div>
      <div v-if="form.weeklySummaryEnabled" class="grid grid-cols-2 gap-3">
        <div class="grid gap-2"><label for="eng-weekday">วันที่ส่ง</label><Select input-id="eng-weekday" v-model="form.weeklySummaryWeekday" :options="weekdays" option-label="label" option-value="value" fluid /></div>
        <div class="grid gap-2"><label for="eng-hour">เวลา (เวลาไทย)</label><Select input-id="eng-hour" v-model="form.weeklySummaryHour" :options="hours" option-label="label" option-value="value" fluid /></div>
      </div>
      <div class="grid gap-2 content-start">
        <label for="eng-inactive">แจ้งผู้ดูแลเมื่อเจ้าของเงียบไป</label>
        <div class="flex items-center gap-3"><ToggleSwitch input-id="eng-inactive" v-model="form.inactivityNoticeEnabled" /><span>{{ form.inactivityNoticeEnabled ? 'เปิด' : 'ปิด' }}</span></div>
      </div>
      <div v-if="form.inactivityNoticeEnabled" class="grid gap-2">
        <label for="eng-quiet-days">นับว่าเงียบเมื่อไม่ใช้ติดต่อกัน (วัน)</label>
        <InputNumber input-id="eng-quiet-days" v-model="form.inactivityQuietDays" :min="2" :max="60" show-buttons fluid />
      </div>
      <div class="md:col-span-2 flex flex-wrap items-center gap-3">
        <Button type="submit" label="บันทึกการดูแลเจ้าของร้าน" icon="pi pi-save" :loading="saving" :disabled="saving || !dirty" />
        <small v-if="dirty" class="text-orange-600">มีค่าที่ยังไม่ได้บันทึก</small>
      </div>
    </form>
  </section>
</template>
