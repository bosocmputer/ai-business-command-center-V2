import { fileURLToPath, URL } from 'node:url';

import { PrimeVueResolver } from '@primevue/auto-import-resolver';
import tailwindcss from '@tailwindcss/vite';
import vue from '@vitejs/plugin-vue';
import Components from 'unplugin-vue-components/vite';
import { configDefaults, defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [
    vue(),
    tailwindcss(),
    Components({ resolvers: [PrimeVueResolver()] })
  ],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  optimizeDeps: {
    include: [
      '@line/liff', '@primeuix/themes/aura',
      'primevue/autocomplete',
      'primevue/badge', 'primevue/button', 'primevue/column', 'primevue/config',
      'primevue/confirmdialog', 'primevue/confirmationservice', 'primevue/datatable',
      'primevue/datepicker', 'primevue/dialog', 'primevue/iconfield', 'primevue/inputgroup',
      'primevue/inputicon', 'primevue/inputmask', 'primevue/inputtext', 'primevue/message',
      'primevue/multiselect', 'primevue/password', 'primevue/progressbar',
      'primevue/progressspinner', 'primevue/select', 'primevue/skeleton', 'primevue/tab',
      'primevue/tablist', 'primevue/tabpanel', 'primevue/tabpanels', 'primevue/tabs',
      'primevue/tag', 'primevue/toast', 'primevue/toastservice', 'primevue/toolbar', 'primevue/tooltip', 'primevue/usetoast'
    ]
  },
  server: {
    port: 5173,
    proxy: { '/api': { target: 'http://127.0.0.1:8080', changeOrigin: false } }
  },
  build: {
    target: 'es2022',
    sourcemap: false,
    chunkSizeWarningLimit: 700
  },
  test: { environment: 'jsdom', setupFiles: ['./src/test/setup.ts'], exclude: [...configDefaults.exclude, 'e2e/**'] }
});
