<script setup lang="ts">
import { computed, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useViewerSession } from '@/stores/viewer';

const router = useRouter();
const { state } = useViewerSession();
const search = ref('');
const openingTenantId = ref('');

const filteredTenants = computed(() => {
  const query = search.value.trim().toLocaleLowerCase('th-TH');
  if (!query) return state.tenants;
  return state.tenants.filter((tenant) => tenant.name.toLocaleLowerCase('th-TH').includes(query));
});

async function openTenant(tenantId: string) {
  if (openingTenantId.value) return;
  openingTenantId.value = tenantId;
  try { await router.push(`/app/tenant/${tenantId}`); }
  finally { openingTenantId.value = ''; }
}
</script>

<template>
  <section v-if="!state.tenants.length" class="card text-center home-empty-state">
    <i class="pi pi-building text-4xl text-muted-color" />
    <h1 class="text-xl mt-4 mb-2">ยังไม่มีร้านค้าที่เข้าถึงได้</h1>
    <p class="text-muted-color mb-0">ติดต่อผู้ดูแลเพื่อเชื่อม LINE บัญชีนี้กับร้านค้า</p>
  </section>

  <section v-else class="card tenant-chooser" aria-labelledby="tenant-chooser-title">
    <div class="tenant-chooser-heading">
      <div>
        <h1 id="tenant-chooser-title" class="text-2xl font-semibold m-0">เลือกร้านที่ต้องการดู</h1>
        <p class="text-muted-color mt-2 mb-0">ระบบจะเปิดข้อมูลหลังจากคุณเลือกร้าน และไม่เลือกร้านแรกให้โดยอัตโนมัติ</p>
      </div>
      <Tag severity="info" :value="`${state.tenants.length.toLocaleString('th-TH')} ร้าน`" />
    </div>

    <IconField v-if="state.tenants.length > 4" class="mt-5">
      <InputIcon class="pi pi-search" />
      <InputText v-model="search" aria-label="ค้นหาชื่อร้าน" placeholder="ค้นหาชื่อร้าน" fluid />
    </IconField>

    <div v-if="filteredTenants.length" class="tenant-options mt-5">
      <Button
        v-for="tenant in filteredTenants"
        :key="tenant.id"
        type="button"
        severity="secondary"
        outlined
        class="tenant-option"
        :loading="openingTenantId === tenant.id"
        :disabled="!!openingTenantId"
        :aria-label="tenant.reportKeys.length
          ? `เปิด Dashboard ร้าน ${tenant.name} มีสิทธิ์ ${tenant.reportKeys.length.toLocaleString('th-TH')} รายงาน`
          : `เปิด Dashboard ร้าน ${tenant.name} ยังไม่ได้กำหนดสิทธิ์รายงาน`"
        @click="openTenant(tenant.id)"
      >
        <span class="tenant-option-content">
          <span class="tenant-icon"><i class="pi pi-building" /></span>
          <span class="tenant-copy">
            <span class="tenant-name">{{ tenant.name }}</span>
            <span v-if="tenant.reportKeys.length" class="tenant-access-summary">
              เปิดดูได้ {{ tenant.reportKeys.length.toLocaleString('th-TH') }} รายงาน
            </span>
            <span v-else class="tenant-pending-summary">
              <span>เข้าร่วมร้านแล้ว</span>
              <Tag severity="warn" value="ยังไม่ได้กำหนดสิทธิ์รายงาน" />
            </span>
          </span>
          <i class="pi pi-arrow-right" aria-hidden="true" />
        </span>
      </Button>
    </div>
    <Message v-else severity="info" :closable="false" class="mt-5">ไม่พบร้านที่ตรงกับคำค้น</Message>
  </section>
</template>

<style scoped>
.home-empty-state,
.tenant-chooser { max-width: 56rem; margin-inline: auto; }
.home-empty-state { padding-block: 4rem; }
.tenant-chooser-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 1rem; }
.tenant-options { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 1rem; }
.tenant-option { min-height: 4.75rem; justify-content: stretch; text-align: left; }
.tenant-option-content { width: 100%; display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: .85rem; }
.tenant-icon { width: 2.75rem; height: 2.75rem; display: grid; place-items: center; border-radius: var(--content-border-radius); color: var(--p-primary-color); background: var(--p-primary-50); }
.tenant-copy { min-width: 0; display: grid; justify-items: start; gap: .35rem; }
.tenant-name { min-width: 0; overflow-wrap: anywhere; font-weight: 600; color: var(--text-color); }
.tenant-access-summary { color: var(--text-color-secondary); font-size: .8rem; }
.tenant-pending-summary { display: flex; flex-wrap: wrap; align-items: center; gap: .4rem .6rem; color: var(--text-color-secondary); font-size: .8rem; }
@media (max-width: 575px) {
  .tenant-chooser { padding: 1.25rem; }
  .tenant-chooser-heading { display: grid; justify-content: stretch; }
  .tenant-chooser-heading :deep(.p-tag) { justify-self: start; }
  .tenant-options { grid-template-columns: 1fr; }
}
</style>
