<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{ enabled: boolean; status: 'PENDING' | 'ACTIVE' | 'REVOKED'; saving: boolean }>();
const emit = defineEmits<{ change: [enabled: boolean] }>();

// A recipient who has not confirmed LINE has no identity the assistant could
// recognise, so the switch stays off until they join.
const needsLineConfirmation = computed(() => props.status !== 'ACTIVE');
const disabled = computed(() => props.saving || needsLineConfirmation.value);
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
    <Message severity="info" :closable="false" class="mt-4 mb-0">ตอนนี้ยังไม่มีผู้ช่วย AI เปิดให้ใช้งาน การตั้งค่านี้เก็บไว้ล่วงหน้าและจะมีผลเมื่อเปิดผู้ช่วย ค่าเริ่มต้นคือปิด</Message>
    <p v-if="needsLineConfirmation" class="ai-chat-hint text-sm text-muted-color mt-3 mb-0">ผู้รับต้องยืนยัน LINE ก่อน จึงจะเปิดสิทธิ์นี้ได้</p>
  </section>
</template>
