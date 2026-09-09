<script setup lang="ts">
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';
import { adminApi } from '@/api';
import { useAdminSession } from '@/stores/session';
import { errorMessage } from '@/utils/format';
import { hasMinimumAdminPasswordLength, minimumAdminPasswordCharacters } from '@/utils/passwordPolicy';

const router = useRouter();
const toast = useToast();
const { updateSession } = useAdminSession();
const currentPassword = ref('');
const newPassword = ref('');
const confirmation = ref('');
const loading = ref(false);
const error = ref('');

async function submit() {
  if (loading.value) return;
  error.value = '';
  if (!currentPassword.value || !hasMinimumAdminPasswordLength(newPassword.value) || newPassword.value !== confirmation.value) {
    error.value = `กรอกรหัสผ่านปัจจุบัน และตั้งรหัสผ่านใหม่อย่างน้อย ${minimumAdminPasswordCharacters} ตัวอักษรโดยยืนยันให้ตรงกัน`;
    return;
  }
  loading.value = true;
  try {
    const session = await adminApi.rotatePassword(currentPassword.value, newPassword.value);
    updateSession(session);
    toast.add({ severity: 'success', summary: 'เปลี่ยนรหัสผ่านแล้ว', life: 3000 });
    await router.replace('/admin');
  } catch (cause) { error.value = errorMessage(cause); }
  finally { loading.value = false; }
}
</script>

<template>
  <div class="max-w-2xl mx-auto">
    <AppPageHeader title="ตั้งรหัสผ่านใหม่" subtitle="ต้องเปลี่ยน bootstrap password ก่อนใช้งานส่วนอื่น" />
    <div class="card">
      <Message severity="warn" :closable="false" class="mb-5">ใช้รหัสผ่านที่ไม่ซ้ำกับระบบอื่นและเก็บใน password manager</Message>
      <Message v-if="error" severity="error" :closable="false" class="mb-5">{{ error }}</Message>
      <form class="grid gap-5" @submit.prevent="submit">
        <div class="grid gap-2"><label for="current">รหัสผ่านปัจจุบัน</label><Password input-id="current" v-model="currentPassword" :feedback="false" toggle-mask fluid /></div>
        <div class="grid gap-2"><label for="new">รหัสผ่านใหม่ (อย่างน้อย {{ minimumAdminPasswordCharacters }} ตัว)</label><Password input-id="new" v-model="newPassword" toggle-mask fluid /></div>
        <div class="grid gap-2"><label for="confirm">ยืนยันรหัสผ่านใหม่</label><Password input-id="confirm" v-model="confirmation" :feedback="false" toggle-mask fluid /></div>
        <Button type="submit" label="บันทึกรหัสผ่าน" icon="pi pi-shield" :loading="loading" :disabled="loading" />
      </form>
    </div>
  </div>
</template>
