<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { useToast } from 'primevue/usetoast';
import { adminApi, type AssistantInstance, type AssistantInstanceOverview, type Recipient } from '@/api';
import { errorMessage, formatDateTime } from '@/utils/format';
import { assistantProblemMessage, instanceIsChanging, instanceStatusLabel, instanceStatusSeverity } from '@/utils/assistantPresentation';

const props = defineProps<{ tenantId: string }>();
const emit = defineEmits<{ changed: [] }>();
const toast = useToast();

const overview = ref<AssistantInstanceOverview>();
const recipients = ref<Recipient[]>([]);
const loading = ref(true);
const loadError = ref('');
const dialog = reactive({ open: false, mode: 'create' as 'create' | 'stop', recipientId: '', namesVisible: false, password: '', busy: false, error: '' });
let timer: ReturnType<typeof setInterval> | undefined;
let lastStatus = '';

const instance = computed<AssistantInstance | null>(() => overview.value?.instance ?? null);
const recipientOptions = computed(() => recipients.value.filter((recipient) => recipient.status === 'ACTIVE' && recipient.aiChatEnabled).map((recipient) => ({ id: recipient.id, label: recipient.displayName || 'ผู้รับ (ยังไม่มีชื่อ)' })));
const canStart = computed(() => !instance.value || instance.value.status === 'STOPPED' || instance.value.status === 'ERROR');
const canStop = computed(() => Boolean(instance.value) && !canStart.value && instance.value?.status !== 'STOPPING');

async function refresh(quiet = false) {
  try {
    overview.value = await adminApi.getAssistantInstance(props.tenantId);
    loadError.value = '';
    const status = overview.value.instance?.status ?? '';
    if (status !== lastStatus && lastStatus !== '' && (status === 'RUNNING' || status === 'STOPPED')) emit('changed');
    lastStatus = status;
    schedule();
  } catch (error) {
    if (!quiet) loadError.value = errorMessage(error);
  } finally {
    loading.value = false;
  }
}

function schedule() {
  const changing = instance.value ? instanceIsChanging(instance.value.status) : false;
  if (changing && !timer) timer = setInterval(() => void refresh(true), 5000);
  if (!changing && timer) { clearInterval(timer); timer = undefined; }
}

async function loadRecipients() {
  try {
    const page = await adminApi.listRecipients(props.tenantId);
    recipients.value = page.data;
  } catch {
    recipients.value = [];
  }
}

function openDialog(mode: 'create' | 'stop') {
  Object.assign(dialog, { open: true, mode, recipientId: recipientOptions.value[0]?.id ?? '', namesVisible: false, password: '', busy: false, error: '' });
}

function closeDialog() {
  Object.assign(dialog, { open: false, password: '', error: '' });
}

async function submit() {
  dialog.busy = true;
  dialog.error = '';
  try {
    overview.value = dialog.mode === 'create'
      ? await adminApi.createAssistantInstance(props.tenantId, dialog.recipientId, dialog.namesVisible, dialog.password)
      : await adminApi.stopAssistantInstance(props.tenantId, dialog.password);
    lastStatus = overview.value.instance?.status ?? '';
    schedule();
    toast.add({ severity: 'success', summary: dialog.mode === 'create' ? 'ส่งคำสั่งสร้างเลขา AI แล้ว' : 'ส่งคำสั่งหยุดแล้ว', detail: 'เซิร์ฟเวอร์จะทำให้ภายใน 1 นาที สถานะด้านล่างจะอัปเดตเอง', life: 6000 });
    emit('changed');
    closeDialog();
  } catch (error) {
    dialog.error = assistantProblemMessage(error);
    dialog.password = '';
  } finally {
    dialog.busy = false;
  }
}

onMounted(async () => {
  await Promise.all([refresh(), loadRecipients()]);
});
onBeforeUnmount(() => { if (timer) clearInterval(timer); });
</script>

<template>
  <section class="grid gap-3" aria-labelledby="assistant-machine">
    <h3 id="assistant-machine" class="text-lg font-semibold m-0">เครื่องของเลขา AI</h3>
    <div v-if="loading" class="flex items-center gap-3"><ProgressSpinner style="width: 1.25rem; height: 1.25rem" /> กำลังโหลด</div>
    <Message v-else-if="loadError" severity="error" :closable="false">{{ loadError }} <Button label="ลองใหม่" text @click="refresh()" /></Message>
    <template v-else>
      <div v-if="instance" class="grid gap-2">
        <div class="flex flex-wrap items-center gap-3">
          <Tag :severity="instanceStatusSeverity(instance.status)" :value="instanceStatusLabel(instance.status, instance.statusCode)" />
          <small class="text-muted-color">ชื่อบริการ <code>{{ instance.host }}</code> · อัปเดต {{ formatDateTime(instance.reportedAt ?? instance.updatedAt) }}</small>
        </div>
        <small v-if="instance.status === 'ERROR'" class="text-muted-color">กด “เริ่มอีกครั้ง” เพื่อลองใหม่ ระบบจะออกรหัสให้เลขาใหม่เอง ถ้ายังผิดพลาดซ้ำ ให้แจ้งผู้ดูแลเซิร์ฟเวอร์พร้อมรหัสข้างต้น</small>
      </div>
      <Message v-else-if="overview?.manualHost" severity="info" :closable="false">เลขา AI ของร้านนี้ถูกตั้งไว้ด้วยมือบนเซิร์ฟเวอร์ (ชื่อบริการ <code>{{ overview.manualHost }}</code>) จึงสร้างหรือหยุดจากหน้านี้ไม่ได้</Message>
      <p v-else class="m-0 text-muted-color">ร้านนี้ยังไม่มีเลขา AI กด “สร้างเลขา AI” แล้วระบบจะสร้างให้เองภายในไม่กี่นาที ไม่ต้องใช้ ssh เมื่อสร้างเสร็จให้ใส่ OpenRouter key และเปิดใช้งานด้านล่าง</p>
      <div v-if="!overview?.manualHost" class="flex flex-wrap gap-2">
        <Button v-if="canStart" :label="instance ? 'เริ่มอีกครั้ง' : 'สร้างเลขา AI'" icon="pi pi-plus" @click="openDialog('create')" />
        <Button v-if="canStop" label="หยุดเลขา AI" severity="secondary" outlined icon="pi pi-stop" @click="openDialog('stop')" />
      </div>
    </template>

    <Dialog v-model:visible="dialog.open" modal :header="dialog.mode === 'create' ? 'สร้างเลขา AI ของร้านนี้' : 'หยุดเลขา AI ของร้านนี้'" :style="{ width: '32rem', maxWidth: '95vw' }" :closable="!dialog.busy" @hide="closeDialog">
      <form class="grid gap-4" autocomplete="off" @submit.prevent="submit">
        <template v-if="dialog.mode === 'create'">
          <div class="grid gap-2">
            <label for="instance-recipient">เลขาจะตอบด้วยสิทธิ์ของใคร</label>
            <Select v-model="dialog.recipientId" input-id="instance-recipient" :options="recipientOptions" option-label="label" option-value="id" placeholder="เลือกผู้รับ" fluid />
            <small class="text-muted-color">เลือกได้เฉพาะผู้รับที่ยืนยันแล้วและเปิดสิทธิ์ “คุยกับผู้ช่วย AI” (เลขาเห็นรายงานเท่ากับคนที่เลือกนี้)</small>
          </div>
          <div class="flex items-center gap-3">
            <ToggleSwitch v-model="dialog.namesVisible" input-id="instance-names" />
            <label for="instance-names">ให้เลขาเห็นชื่อลูกค้าและผู้จำหน่ายจริง (ปิดไว้ = แสดงเป็นรหัสแทนชื่อ)</label>
          </div>
          <Message v-if="!recipientOptions.length" severity="warn" :closable="false">ยังไม่มีผู้รับที่เปิดสิทธิ์คุยกับผู้ช่วย AI ไปเปิดที่หน้ากำหนดสิทธิ์ของผู้รับก่อน</Message>
        </template>
        <Message v-else severity="warn" :closable="false">เลขาจะหยุดตอบทุกช่องทาง (LINE และ Telegram) ค่าที่ตั้งไว้ไม่หาย เริ่มใหม่ได้ภายหลัง</Message>
        <div class="grid gap-2">
          <label for="instance-password">รหัสผ่าน admin (ยืนยันอีกครั้ง)</label>
          <InputText id="instance-password" v-model="dialog.password" type="password" autocomplete="current-password" fluid />
        </div>
        <Message v-if="dialog.error" severity="error" :closable="false">{{ dialog.error }}</Message>
        <div class="flex justify-end gap-2">
          <Button type="button" label="ยกเลิก" text :disabled="dialog.busy" @click="closeDialog" />
          <Button type="submit" :label="dialog.mode === 'create' ? 'สร้าง' : 'หยุด'" :severity="dialog.mode === 'create' ? undefined : 'danger'" :loading="dialog.busy" :disabled="dialog.busy || !dialog.password || (dialog.mode === 'create' && !dialog.recipientId)" />
        </div>
      </form>
    </Dialog>
  </section>
</template>
