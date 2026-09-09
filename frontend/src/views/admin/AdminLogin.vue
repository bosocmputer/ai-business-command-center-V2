<script setup lang="ts">
import { ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useAdminSession } from '@/stores/session';
import { errorMessage } from '@/utils/format';
import { hasAdminLoginInput } from '@/utils/passwordPolicy';

const route = useRoute();
const router = useRouter();
const { login } = useAdminSession();
const username = ref('superadmin');
const password = ref('');
const loading = ref(false);
const error = ref(route.query.expired === '1' ? 'Session หมดอายุ กรุณาเข้าสู่ระบบใหม่' : route.query.sessionError === '1' ? 'ไม่สามารถตรวจสอบ Session ได้ กรุณาเข้าสู่ระบบใหม่' : '');

async function submit() {
  error.value = '';
  if (!hasAdminLoginInput(username.value, password.value)) {
    error.value = 'กรุณากรอก username และรหัสผ่าน';
    return;
  }
  loading.value = true;
  try {
    const session = await login(username.value.trim(), password.value);
    const requestedRedirect = typeof route.query.redirect === 'string' ? route.query.redirect : '';
    const redirect = requestedRedirect === '/admin' || requestedRedirect.startsWith('/admin/') ? requestedRedirect : '/admin';
    await router.replace(session.mustRotateBootstrapPassword ? '/admin/password' : redirect);
  } catch (cause) {
    error.value = errorMessage(cause);
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <main class="bg-surface-50 dark:bg-surface-950 flex items-center justify-center min-h-screen min-w-full overflow-hidden px-4 py-8">
    <div class="login-frame w-full max-w-[38rem]">
      <section class="login-card w-full bg-surface-0 dark:bg-surface-900 py-12 px-6 sm:py-16 sm:px-16">
        <div class="text-center mb-8">
          <div class="login-mark grid place-items-center w-16 h-16 bg-primary text-primary-contrast mx-auto mb-6"><i class="pi pi-chart-line text-2xl" /></div>
          <h1 class="login-title m-0 font-bold">Nextstep Admin</h1>
          <p class="m-0 mt-3 text-muted-color font-medium">เข้าสู่ระบบผู้ดูแลส่วนกลาง</p>
        </div>
        <Message v-if="error" severity="error" :closable="false" class="mb-6" role="alert">{{ error }}</Message>
        <form class="grid gap-6" @submit.prevent="submit">
          <div class="grid gap-2"><label for="username" class="font-medium text-lg">ชื่อผู้ใช้</label><InputText id="username" v-model="username" autocomplete="username" size="large" fluid /></div>
          <div class="grid gap-2"><label for="password" class="font-medium text-lg">รหัสผ่าน</label><Password input-id="password" v-model="password" :feedback="false" toggle-mask autocomplete="current-password" size="large" fluid /></div>
          <Button type="submit" label="เข้าสู่ระบบ" icon="pi pi-sign-in" :loading="loading" :disabled="loading" size="large" fluid />
        </form>
        <p class="text-sm text-muted-color text-center mt-7 mb-0">ระบบจะล็อกชั่วคราวเมื่อกรอกรหัสผ่านผิดหลายครั้ง</p>
      </section>
    </div>
  </main>
</template>

<style scoped>
.login-frame { border-radius: 56px; padding: .3rem; background: linear-gradient(180deg, var(--primary-color) 10%, color-mix(in srgb, var(--primary-color) 0%, transparent) 30%); }
.login-card { border-radius: 53px; }
.login-mark { border-radius: var(--content-border-radius); }
.login-title { font-size: 2rem; }
</style>
