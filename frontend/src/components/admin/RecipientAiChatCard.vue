<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import type { AgentTokenStatus } from '@/api';

const props = withDefaults(defineProps<{
  enabled: boolean;
  status: 'PENDING' | 'ACTIVE' | 'REVOKED';
  saving: boolean;
  agent?: AgentTokenStatus;
  agentBusy?: boolean;
  issuedToken?: string;
  now?: Date;
}>(), { agent: undefined, agentBusy: false, issuedToken: undefined, now: () => new Date() });
const emit = defineEmits<{ change: [enabled: boolean]; issue: [namesVisible: boolean]; revoke: []; dismissToken: [] }>();

// A recipient who has not confirmed LINE has no identity the assistant could
// recognise, so the switch stays off until they join.
const needsLineConfirmation = computed(() => props.status !== 'ACTIVE');
const disabled = computed(() => props.saving || needsLineConfirmation.value);

const namesVisible = ref(false);
const confirming = ref<'issue' | 'revoke' | undefined>();
const copied = ref(false);
watch(() => props.agent?.info.namesVisible, (value) => { if (value !== undefined) namesVisible.value = value; }, { immediate: true });
watch(() => props.issuedToken, () => { copied.value = false; });

const info = computed(() => props.agent?.info);
const systemOff = computed(() => props.agent !== undefined && !props.agent.enabled);
const hasToken = computed(() => info.value?.status === 'ACTIVE' || info.value?.status === 'EXPIRED');
const daysLeft = computed(() => {
  if (!info.value?.expiresAt) return undefined;
  return Math.ceil((Date.parse(info.value.expiresAt) - props.now.getTime()) / 86_400_000);
});
const expiringSoon = computed(() => info.value?.status === 'ACTIVE' && daysLeft.value !== undefined && daysLeft.value <= 14);

function thaiDate(value?: string): string {
  if (!value) return 'ยังไม่เคยใช้';
  return new Intl.DateTimeFormat('th-TH', { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit', timeZone: 'Asia/Bangkok' }).format(new Date(value)) + ' น.';
}

const statusText = computed(() => {
  if (!info.value || info.value.status === 'NONE') return 'ยังไม่มีโทเคน ผู้ช่วยยังเข้าถึงข้อมูลร้านไม่ได้';
  if (info.value.status === 'EXPIRED') return 'โทเคนหมดอายุแล้ว ผู้ช่วยเข้าถึงข้อมูลไม่ได้ ต้องออกใหม่';
  return `ใช้งานได้ · หมดอายุ ${thaiDate(info.value.expiresAt)} (อีก ${daysLeft.value} วัน)`;
});

function askIssue() {
  if (!hasToken.value) { emit('issue', namesVisible.value); return; }
  confirming.value = 'issue';
}
function confirmed() {
  const action = confirming.value;
  confirming.value = undefined;
  if (action === 'issue') emit('issue', namesVisible.value);
  if (action === 'revoke') emit('revoke');
}
async function copyToken() {
  if (!props.issuedToken) return;
  try { await navigator.clipboard.writeText(props.issuedToken); copied.value = true; }
  catch { copied.value = false; }
}
</script>

<template>
  <section class="card" aria-labelledby="ai-chat-title">
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div class="min-w-0" style="flex: 1 1 22rem">
        <h2 id="ai-chat-title" class="text-lg font-semibold m-0">คุยกับผู้ช่วย AI</h2>
        <p class="text-muted-color mt-1 mb-0">อนุญาตให้ผู้รับคนนี้ถามข้อมูลร้านกับผู้ช่วย AI ได้ ผู้ช่วยตอบได้เฉพาะรายงานที่ได้รับสิทธิ์ข้างต้น</p>
      </div>
      <div class="flex items-center gap-3">
        <label for="ai-chat-switch" class="ai-chat-state">{{ enabled ? 'เปิด' : 'ปิด' }}</label>
        <ToggleSwitch input-id="ai-chat-switch" :model-value="enabled" :disabled="disabled" @update:model-value="emit('change', Boolean($event))" />
      </div>
    </div>
    <p v-if="needsLineConfirmation" class="ai-chat-hint text-sm text-muted-color mt-3 mb-0">ผู้รับต้องยืนยัน LINE ก่อน จึงจะเปิดสิทธิ์นี้ได้</p>

    <Message v-if="systemOff" severity="warn" :closable="false" class="ai-chat-system-off mt-4 mb-0">ผู้ช่วย AI ยังปิดอยู่ทั้งระบบ จึงยังออกโทเคนไม่ได้ ผู้ดูแลเครื่องต้องตั้งค่า <code>AGENT_API_ENABLED=true</code> ก่อน</Message>
    <Message v-else-if="!agent" severity="info" :closable="false" class="mt-4 mb-0">สวิตช์นี้เก็บไว้ล่วงหน้า ผู้ช่วยจะใช้ได้เมื่อมีโทเคน</Message>

    <div v-if="agent && !systemOff && enabled" class="ai-chat-token mt-4 grid gap-3">
      <div>
        <h3 class="text-base font-semibold m-0">โทเคนของผู้ช่วย</h3>
        <p class="ai-chat-token-status m-0 mt-1" :class="info?.status === 'ACTIVE' ? '' : 'text-muted-color'">{{ statusText }}</p>
        <p v-if="info?.status === 'ACTIVE' || info?.status === 'EXPIRED'" class="text-sm text-muted-color m-0 mt-1">ใช้ล่าสุด {{ thaiDate(info.lastUsedAt) }} · 24 ชั่วโมงที่ผ่านมา {{ info.calls24h }} คำขอ</p>
        <Message v-if="expiringSoon" severity="warn" :closable="false" class="ai-chat-expiring mt-2 mb-0">โทเคนจะหมดอายุในอีก {{ daysLeft }} วัน ควรออกใหม่ก่อนหมด เพื่อให้ผู้ช่วยไม่หยุดทำงาน</Message>
      </div>

      <div class="flex items-start gap-3">
        <ToggleSwitch input-id="ai-names-switch" v-model="namesVisible" :disabled="agentBusy" />
        <label for="ai-names-switch" class="min-w-0">
          <span class="font-medium">ให้ผู้ช่วยเห็นชื่อลูกค้าและผู้จำหน่าย</span>
          <span class="block text-sm text-muted-color">ปิด: ผู้ช่วยเห็นเป็นรหัส เช่น “ลูกค้า-7F3A” เปิด: เห็นชื่อจริง ซึ่งจะถูกส่งให้ผู้ให้บริการโมเดล AI ด้วย ตรวจข้อตกลงกับร้านก่อนเปิด การเปลี่ยนค่านี้มีผลเมื่อออกโทเคนใหม่</span>
        </label>
      </div>

      <div v-if="confirming" class="ai-chat-confirm flex flex-wrap items-center gap-3">
        <span>{{ confirming === 'issue' ? 'โทเคนเดิมจะใช้ไม่ได้ทันที ผู้ช่วยที่ใช้โทเคนเดิมอยู่จะหยุดตอบจนกว่าจะใส่โทเคนใหม่' : 'ผู้ช่วยจะเข้าถึงข้อมูลร้านไม่ได้ทันที' }}</span>
        <Button :label="confirming === 'issue' ? 'ออกโทเคนใหม่' : 'ยกเลิกโทเคน'" :severity="confirming === 'revoke' ? 'danger' : undefined" :loading="agentBusy" class="touch-action" @click="confirmed" />
        <Button label="ไม่ทำ" text class="touch-action" @click="confirming = undefined" />
      </div>
      <div v-else class="flex flex-wrap gap-2">
        <Button :label="hasToken ? 'ออกโทเคนใหม่' : 'ออกโทเคน'" icon="pi pi-key" :loading="agentBusy" class="touch-action" @click="askIssue" />
        <Button v-if="hasToken" label="ยกเลิกโทเคน" icon="pi pi-ban" severity="danger" outlined :disabled="agentBusy" class="touch-action" @click="confirming = 'revoke'" />
      </div>

      <div v-if="issuedToken" class="ai-chat-issued" role="status">
        <Message severity="success" :closable="false" class="mb-2">ออกโทเคนแล้ว โทเคนนี้แสดงครั้งเดียว ปิดแล้วดูอีกไม่ได้ ถ้าหายต้องออกใหม่</Message>
        <div class="flex flex-wrap items-center gap-2">
          <input class="ai-chat-token-value p-inputtext p-component" style="flex: 1 1 18rem; font-family: monospace" readonly :value="issuedToken" aria-label="โทเคนผู้ช่วยใหม่" @focus="($event.target as HTMLInputElement).select()" />
          <Button :label="copied ? 'คัดลอกแล้ว' : 'คัดลอก'" :icon="copied ? 'pi pi-check' : 'pi pi-copy'" class="touch-action" @click="copyToken" />
          <Button label="ปิด" text class="touch-action" @click="emit('dismissToken')" />
        </div>
        <p class="text-sm text-muted-color mt-2 mb-0">นำไปใส่เป็นค่า <code>AIBCC_TOKEN</code> ในการตั้งค่าของผู้ช่วย ห้ามส่งทางแชตหรืออีเมล และห้ามวางในที่สาธารณะ</p>
      </div>
    </div>
  </section>
</template>
