<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useToast } from 'primevue/usetoast';
import { adminApi, ApiError, type AssistantGlobalSettings, type AssistantModel, type AssistantSecretField, type AssistantSettings } from '@/api';
import { errorMessage, formatDateTime } from '@/utils/format';
import { assistantProblemMessage, modelStatusLabel, modelStatusSeverity, secretLabel } from '@/utils/assistantPresentation';

const props = defineProps<{ tenantId: string }>();
const toast = useToast();

const settings = ref<AssistantSettings>();
const shared = ref<AssistantGlobalSettings>();
const loading = ref(true);
const loadError = ref('');
const saving = ref(false);
const form = reactive({ enabled: false, isTest: false, modelKey: '', lineMode: 'NONE' as 'NONE' | 'CENTRAL' | 'OWN', assistantHost: '' });

type SecretTarget = { kind: 'tenant' | 'global'; field: AssistantSecretField; clear: boolean };
const dialog = reactive({ open: false, kind: 'tenant' as SecretTarget['kind'], field: 'openrouter-key' as AssistantSecretField, clear: false, value: '', password: '', busy: false, error: '' });

const lineModes = [
  { value: 'NONE', label: 'ไม่ใช้ LINE' },
  { value: 'CENTRAL', label: 'ช่อง LINE กลาง' },
  { value: 'OWN', label: 'ช่อง LINE ของร้านเอง' }
];

const secretRows: { field: AssistantSecretField; key: 'openrouterKey' | 'telegramBotToken' | 'lineChannelSecret' | 'lineChannelToken'; hint: string }[] = [
  { field: 'openrouter-key', key: 'openrouterKey', hint: 'บัญชี OpenRouter ที่ร้านนี้ใช้จ่ายค่าโมเดล' },
  { field: 'telegram-bot-token', key: 'telegramBotToken', hint: 'บอท Telegram ของร้าน (ถ้าใช้)' },
  { field: 'line-channel-secret', key: 'lineChannelSecret', hint: 'ใช้เมื่อเลือกช่อง LINE ของร้านเอง' },
  { field: 'line-channel-token', key: 'lineChannelToken', hint: 'ใช้เมื่อเลือกช่อง LINE ของร้านเอง' }
];

const modelOptions = computed(() => (settings.value?.models ?? []).map((model) => ({ ...model, disabled: !model.selectable })));
const selectedModel = computed<AssistantModel | undefined>(() => settings.value?.models.find((model) => model.key === form.modelKey));
const dirty = computed(() => Boolean(settings.value) && (form.enabled !== settings.value!.enabled || form.isTest !== settings.value!.isTest || form.modelKey !== settings.value!.modelKey || form.lineMode !== settings.value!.lineMode || form.assistantHost !== settings.value!.assistantHost));
const status = computed(() => settings.value?.status);
const dialogTitle = computed(() => `${dialog.clear ? 'ลบ' : 'ตั้ง'} ${secretLabel(dialog.field)}${dialog.kind === 'global' ? ' (ช่อง LINE กลาง)' : ''}`);

function adopt(next: AssistantSettings) {
  settings.value = next;
  Object.assign(form, { enabled: next.enabled, isTest: next.isTest, modelKey: next.modelKey, lineMode: next.lineMode, assistantHost: next.assistantHost });
}

async function load() {
  loading.value = true;
  loadError.value = '';
  try {
    const [loaded, sharedSettings] = await Promise.all([adminApi.getAssistant(props.tenantId), adminApi.getAssistantGlobal()]);
    adopt(loaded);
    shared.value = sharedSettings;
  } catch (error) {
    loadError.value = errorMessage(error);
  } finally {
    loading.value = false;
  }
}

async function save() {
  if (!settings.value) return;
  saving.value = true;
  try {
    adopt(await adminApi.updateAssistant(props.tenantId, { enabled: form.enabled, isTest: form.isTest, modelKey: form.modelKey, lineMode: form.lineMode, assistantHost: form.assistantHost.trim(), version: settings.value.version }));
    toast.add({ severity: 'success', summary: 'บันทึกแล้ว', detail: 'ผู้ช่วยของร้านจะใช้ค่าใหม่เมื่อรีสตาร์ต (ราว 1 นาที เมื่อไม่มีคนคุยค้างอยู่)', life: 5000 });
  } catch (error) {
    toast.add({ severity: 'error', summary: 'บันทึกไม่สำเร็จ', detail: assistantProblemMessage(error), life: 7000 });
    if (error instanceof ApiError && error.code === 'VERSION_CONFLICT') await load();
  } finally {
    saving.value = false;
  }
}

function openDialog(target: SecretTarget) {
  Object.assign(dialog, { open: true, kind: target.kind, field: target.field, clear: target.clear, value: '', password: '', busy: false, error: '' });
}

function closeDialog() {
  // The typed secret and password are wiped as soon as the dialog goes away.
  Object.assign(dialog, { open: false, value: '', password: '', error: '' });
}

async function submitDialog() {
  dialog.busy = true;
  dialog.error = '';
  try {
    if (dialog.kind === 'global') {
      shared.value = await adminApi.setAssistantGlobalSecret(dialog.field as 'line-channel-secret' | 'line-channel-token', dialog.value, dialog.password);
      await load();
    } else if (dialog.clear) {
      adopt(await adminApi.clearAssistantSecret(props.tenantId, dialog.field, dialog.password));
    } else {
      adopt(await adminApi.setAssistantSecret(props.tenantId, dialog.field, dialog.value, dialog.password));
    }
    toast.add({ severity: 'success', summary: dialog.clear ? 'ลบแล้ว' : 'บันทึกรหัสแล้ว', detail: 'ค่าที่เก็บไว้จะไม่แสดงกลับมาอีก', life: 4000 });
    closeDialog();
  } catch (error) {
    dialog.error = assistantProblemMessage(error);
    dialog.password = '';
  } finally {
    dialog.busy = false;
  }
}

onMounted(load);
</script>

<template>
  <div class="grid gap-6 max-w-4xl">
    <div v-if="loading" class="flex items-center gap-3"><ProgressSpinner style="width: 1.5rem; height: 1.5rem" /> กำลังโหลด</div>
    <Message v-else-if="loadError" severity="error" :closable="false">{{ loadError }} <Button label="ลองใหม่" text @click="load" /></Message>
    <template v-else-if="settings">
      <section class="grid gap-3" aria-labelledby="assistant-state">
        <h3 id="assistant-state" class="text-lg font-semibold m-0">สถานะเลขา AI</h3>
        <div class="flex flex-wrap items-center gap-3">
          <ToggleSwitch v-model="form.enabled" input-id="assistant-enabled" />
          <label for="assistant-enabled">เปิดใช้เลขา AI ของร้านนี้</label>
          <Tag v-if="status" :severity="status.upToDate ? 'success' : 'warn'" :value="status.upToDate ? 'ใช้ค่าล่าสุดแล้ว' : `ใช้ค่ารุ่น ${status.appliedConfigVersion} (ล่าสุดคือ ${settings.configVersion}) รอผู้ช่วยรีสตาร์ต`" />
          <Tag v-else severity="secondary" value="ผู้ช่วยยังไม่เคยรายงานสถานะ" />
        </div>
        <small v-if="status" class="text-muted-color">ผู้ช่วยติดต่อล่าสุด {{ formatDateTime(status.lastSeenAt) }}<template v-if="status.reportedModelKey"> · โมเดลที่ใช้อยู่ {{ status.reportedModelKey }}</template><template v-if="status.lastErrorCode"> · ข้อผิดพลาดล่าสุด {{ status.lastErrorCode }}</template></small>
        <small class="text-muted-color">ปิดเมื่อสิ้นสุดสิทธิ์ของร้านโดยอัตโนมัติ (ดูวันสิ้นสุดสิทธิ์ในแท็บข้อมูลร้าน)</small>
      </section>

      <section class="grid gap-3" aria-labelledby="assistant-model">
        <h3 id="assistant-model" class="text-lg font-semibold m-0">โมเดล</h3>
        <div class="flex items-center gap-3"><Checkbox v-model="form.isTest" input-id="assistant-test" binary /><label for="assistant-test">ร้านทดสอบ (เลือกโมเดลสำหรับทดสอบได้ ห้ามใช้กับข้อมูลลูกค้าจริง)</label></div>
        <Select v-model="form.modelKey" :options="modelOptions" option-label="label" option-value="key" option-disabled="disabled" input-id="assistant-model-select" aria-label="โมเดล" class="max-w-md" fluid>
          <template #option="{ option }">
            <div class="flex items-center justify-between gap-3 w-full" :class="{ 'opacity-50': !option.selectable }">
              <span>{{ option.label }}</span>
              <Tag :severity="modelStatusSeverity(option.status)" :value="modelStatusLabel(option.status)" />
            </div>
          </template>
        </Select>
        <div v-if="selectedModel" class="grid gap-2 p-3 border border-surface rounded-border">
          <div class="flex flex-wrap items-center gap-2"><strong>{{ selectedModel.label }}</strong><Tag :severity="modelStatusSeverity(selectedModel.status)" :value="modelStatusLabel(selectedModel.status)" /><Tag v-if="selectedModel.vision" severity="info" value="อ่านรูปได้" /><Tag v-if="selectedModel.default" severity="success" value="ค่าเริ่มต้น" /></div>
          <p class="m-0">{{ selectedModel.summary }}</p>
          <small v-if="selectedModel.measured" class="text-muted-color">ผลทดสอบ 1 คำถาม: ตอบใน {{ selectedModel.measured.seconds }} วินาที ต้นทุน ${{ selectedModel.measured.usd }} ต่อคำถาม (ตัวเลขจากการลองครั้งเดียว ไม่ใช่เกณฑ์มาตรฐาน)</small>
          <small v-if="!selectedModel.selectable" class="text-red-600">โมเดลนี้เลือกได้เฉพาะร้านที่ตั้งเป็นร้านทดสอบ</small>
        </div>
      </section>

      <section class="grid gap-3" aria-labelledby="assistant-line">
        <h3 id="assistant-line" class="text-lg font-semibold m-0">ช่องทาง LINE</h3>
        <SelectButton v-model="form.lineMode" :options="lineModes" option-label="label" option-value="value" aria-label="โหมด LINE" :allow-empty="false" />
        <small v-if="form.lineMode === 'CENTRAL'" class="text-muted-color">ใช้ช่อง LINE กลางของผู้ดูแล <Tag :severity="settings.centralLineConfigured ? 'success' : 'warn'" :value="settings.centralLineConfigured ? 'ตั้งค่าช่องกลางแล้ว' : 'ยังไม่ได้ตั้งค่าช่องกลาง'" /></small>
        <div v-if="form.lineMode === 'CENTRAL'" class="grid gap-1">
          <label for="assistant-host" class="font-medium">ชื่อบริการผู้ช่วยของร้าน (ในเครือข่ายภายใน)</label>
          <InputText id="assistant-host" v-model="form.assistantHost" maxlength="62" autocomplete="off" placeholder="เช่น assistant" aria-describedby="assistant-host-help" />
          <small id="assistant-host-help" class="text-muted-color">ด่านหน้า LINE ใช้ชื่อนี้ส่งข้อความต่อให้ผู้ช่วยของร้าน เว้นว่างไว้ = ข้อความจากช่อง LINE กลางจะไม่ถูกส่งมาที่ร้านนี้ ใช้ตัวอักษรเล็ก a-z ตัวเลข และขีดกลางเท่านั้น</small>
        </div>
        <small v-else-if="form.lineMode === 'OWN'" class="text-muted-color">ใช้ช่อง LINE ของร้านเอง ต้องตั้ง Channel secret และ Channel access token ด้านล่าง</small>
        <small v-else class="text-muted-color">ร้านนี้คุยผ่าน Telegram เท่านั้น</small>
      </section>

      <div class="flex items-center gap-3">
        <Button label="บันทึกการตั้งค่า" icon="pi pi-save" :loading="saving" :disabled="saving || !dirty" @click="save" />
        <small v-if="dirty" class="text-orange-600">มีการแก้ไขที่ยังไม่บันทึก</small>
      </div>

      <section class="grid gap-3" aria-labelledby="assistant-secrets">
        <h3 id="assistant-secrets" class="text-lg font-semibold m-0">รหัสลับของร้าน</h3>
        <small class="text-muted-color">รหัสที่บันทึกแล้วจะไม่แสดงกลับมา ใส่รหัสผ่าน admin ซ้ำทุกครั้งที่ตั้งหรือลบ</small>
        <div v-for="row in secretRows" :key="row.field" class="flex flex-wrap items-center justify-between gap-3 p-3 border border-surface rounded-border">
          <div>
            <div class="font-semibold">{{ secretLabel(row.field) }}</div>
            <small class="text-muted-color">{{ row.hint }}</small>
          </div>
          <div class="flex items-center gap-2">
            <Tag :severity="settings.secrets[row.key].isSet ? 'success' : 'secondary'" :value="settings.secrets[row.key].isSet ? (settings.secrets[row.key].last4 ? `ตั้งแล้ว ลงท้าย ${settings.secrets[row.key].last4}` : 'ตั้งแล้ว') : 'ยังไม่ตั้ง'" />
            <Button :label="settings.secrets[row.key].isSet ? 'เปลี่ยน' : 'ตั้ง'" size="small" outlined @click="openDialog({ kind: 'tenant', field: row.field, clear: false })" />
            <Button v-if="settings.secrets[row.key].isSet" label="ลบ" size="small" severity="danger" text @click="openDialog({ kind: 'tenant', field: row.field, clear: true })" />
          </div>
        </div>
      </section>

      <section v-if="shared" class="grid gap-3" aria-labelledby="assistant-global">
        <h3 id="assistant-global" class="text-lg font-semibold m-0">ช่อง LINE กลางของผู้ดูแล (ใช้ร่วมทุกร้านที่เลือก "ช่อง LINE กลาง")</h3>
        <div class="flex flex-wrap items-center justify-between gap-3 p-3 border border-surface rounded-border">
          <div class="font-semibold">Channel secret</div>
          <div class="flex items-center gap-2"><Tag :severity="shared.lineChannelSecret.isSet ? 'success' : 'secondary'" :value="shared.lineChannelSecret.isSet ? 'ตั้งแล้ว' : 'ยังไม่ตั้ง'" /><Button label="ตั้ง/เปลี่ยน" size="small" outlined @click="openDialog({ kind: 'global', field: 'line-channel-secret', clear: false })" /></div>
        </div>
        <div class="flex flex-wrap items-center justify-between gap-3 p-3 border border-surface rounded-border">
          <div class="font-semibold">Channel access token</div>
          <div class="flex items-center gap-2"><Tag :severity="shared.lineChannelToken.isSet ? 'success' : 'secondary'" :value="shared.lineChannelToken.isSet ? 'ตั้งแล้ว' : 'ยังไม่ตั้ง'" /><Button label="ตั้ง/เปลี่ยน" size="small" outlined @click="openDialog({ kind: 'global', field: 'line-channel-token', clear: false })" /></div>
        </div>
        <small class="text-muted-color">เปลี่ยนค่านี้มีผลกับทุกร้านที่ใช้ช่องกลาง</small>
      </section>
    </template>

    <Dialog v-model:visible="dialog.open" modal :header="dialogTitle" :style="{ width: '32rem', maxWidth: '95vw' }" :closable="!dialog.busy" @hide="closeDialog">
      <form class="grid gap-4" autocomplete="off" @submit.prevent="submitDialog">
        <div v-if="!dialog.clear" class="grid gap-2">
          <label for="assistant-secret-value">ค่าที่จะบันทึก</label>
          <InputText id="assistant-secret-value" v-model="dialog.value" type="password" autocomplete="off" spellcheck="false" fluid />
        </div>
        <Message v-else severity="warn" :closable="false">ลบแล้วผู้ช่วยจะใช้ช่องทางนี้ไม่ได้จนกว่าจะตั้งใหม่</Message>
        <div class="grid gap-2">
          <label for="assistant-admin-password">รหัสผ่าน admin (ยืนยันอีกครั้ง)</label>
          <InputText id="assistant-admin-password" v-model="dialog.password" type="password" autocomplete="current-password" fluid />
        </div>
        <Message v-if="dialog.error" severity="error" :closable="false">{{ dialog.error }}</Message>
        <div class="flex justify-end gap-2">
          <Button type="button" label="ยกเลิก" text :disabled="dialog.busy" @click="closeDialog" />
          <Button type="submit" :label="dialog.clear ? 'ลบ' : 'บันทึก'" :severity="dialog.clear ? 'danger' : undefined" :loading="dialog.busy" :disabled="dialog.busy || !dialog.password || (!dialog.clear && !dialog.value)" />
        </div>
      </form>
    </Dialog>
  </div>
</template>
