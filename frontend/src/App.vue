<script setup lang="ts">
import { onBeforeUnmount, onErrorCaptured, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useAdminSession } from '@/stores/session';

const route = useRoute();
const router = useRouter();
const { clearSession } = useAdminSession();
const fatalError = ref(false);

function handleUnauthorized(event: Event) {
  const scope = (event as CustomEvent<{ scope?: string }>).detail?.scope;
  if (scope !== 'admin' || route.name === 'admin-login') return;
  clearSession();
  const redirect = route.fullPath === '/admin' || route.fullPath.startsWith('/admin/') ? route.fullPath : '/admin';
  void router.replace({ name: 'admin-login', query: { redirect, expired: '1' } });
}

function showFatalError(error: unknown) {
  console.error('Unhandled application error', error);
  fatalError.value = true;
}
function reloadPage() { window.location.reload(); }

window.addEventListener('nextstep:unauthorized', handleUnauthorized);
router.onError(showFatalError);
onErrorCaptured((error) => { showFatalError(error); return false; });
onBeforeUnmount(() => window.removeEventListener('nextstep:unauthorized', handleUnauthorized));
</script>

<template>
  <main v-if="fatalError" class="min-h-screen grid place-items-center p-6 bg-surface-50 dark:bg-surface-950">
    <section class="card text-center max-w-lg"><i class="pi pi-exclamation-triangle text-orange-500 text-5xl" /><h1 class="text-2xl mt-5 mb-2">หน้านี้ทำงานไม่สมบูรณ์</h1><p class="text-muted-color">ข้อมูลของคุณไม่ได้ถูกบันทึกจากหน้าที่เกิดปัญหา กรุณาโหลดหน้าใหม่แล้วลองอีกครั้ง</p><Button label="โหลดหน้าใหม่" icon="pi pi-refresh" @click="reloadPage" /></section>
  </main>
  <RouterView v-else />
</template>
