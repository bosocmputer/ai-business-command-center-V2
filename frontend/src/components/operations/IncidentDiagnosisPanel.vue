<script setup lang="ts">
import { computed } from 'vue';
import type { IncidentDiagnosis, OperationalIncidentOccurrence } from '@/api';
import { formatDateTime } from '@/utils/format';
import { formatDurationMs, lineImpactLabel, reportImpactLabel } from '@/utils/operationalPresentation';
import { isLegacyDiagnosis, javaWSStatusLabel, matchingSuccessLabel, protocolRequestLabel } from '@/utils/incidentDiagnosis';

const props = defineProps<{
  diagnosis?: IncidentDiagnosis;
  occurrence: OperationalIncidentOccurrence;
  reportLabel: string;
  loading?: boolean;
  error?: string;
  embedded?: boolean;
}>();

defineEmits<{
  retry: [];
  copyCustomer: [diagnosis: IncidentDiagnosis];
}>();

const protocol = computed(() => props.diagnosis?.protocolEvidence);
const evidenceDuration = computed(() => props.occurrence.failureEvidence?.durationMs);

function byteLabel(value?: number) {
  if (value === undefined) return 'ไม่ทราบ';
  return `${value.toLocaleString('th-TH')} ไบต์`;
}
</script>

<template>
  <section class="diagnosis-panel" :class="{ card: !embedded }" aria-labelledby="diagnosis-title">
    <div class="diagnosis-context">
      <div>
        <span class="context-eyebrow">เหตุที่ตรวจพบ</span>
        <h2 id="diagnosis-title">{{ occurrence.tenantName || 'ระบบส่วนกลาง' }}</h2>
        <p>{{ reportLabel }} · {{ formatDateTime(occurrence.observedAt) }} เวลาไทย</p>
      </div>
      <Tag :severity="occurrence.failureEvidence?.level === 'CONFIRMED' ? 'info' : 'secondary'" :value="occurrence.failureEvidence?.level === 'CONFIRMED' ? 'มีหลักฐานยืนยัน' : 'หลักฐานบางส่วน'" />
    </div>

    <template v-if="loading">
      <Skeleton height="2rem" width="70%" class="mb-3" />
      <Skeleton height="8rem" />
    </template>
    <Message v-else-if="error" severity="error" :closable="false">
      <div class="flex flex-wrap align-items-center justify-content-between gap-3">
        <span>โหลดสาเหตุและหลักฐานไม่สำเร็จ · {{ error }}</span>
        <Button label="ลองใหม่" icon="pi pi-refresh" size="small" outlined @click="$emit('retry')" />
      </div>
    </Message>
    <template v-else-if="diagnosis">
      <div class="diagnosis-lead">
        <span>เกิดอะไรขึ้น</span>
        <strong>{{ diagnosis.assessment.summaryTh }}</strong>
      </div>

      <div class="diagnosis-facts">
        <div class="fact-card">
          <i class="pi pi-map-marker" aria-hidden="true" />
          <span>เกิดที่ส่วนไหน</span>
          <strong>{{ diagnosis.assessment.problemAreaTh }}</strong>
        </div>
        <div class="fact-card">
          <i class="pi pi-user" aria-hidden="true" />
          <span>ผู้ที่ควรตรวจสอบ</span>
          <strong>{{ diagnosis.assessment.ownerTh }}</strong>
        </div>
        <div class="fact-card">
          <i class="pi pi-gauge" aria-hidden="true" />
          <span>ภาระจาก Nextstep</span>
          <strong>{{ diagnosis.assessment.loadSignalTh }}</strong>
        </div>
      </div>

      <div class="diagnosis-section">
        <h3>สถานะ Java Web Service</h3>
        <Message :severity="protocol?.httpStatus && protocol.httpStatus >= 200 && protocol.httpStatus < 300 ? 'info' : 'warn'" :closable="false" class="m-0">
          {{ javaWSStatusLabel(diagnosis) }}
        </Message>
        <Message v-if="isLegacyDiagnosis(diagnosis)" severity="secondary" :closable="false" class="mt-3 mb-0">หลักฐานจากระบบรุ่นเดิมไม่เพียงพอ ระบบจึงไม่ยืนยันสาเหตุหรือภาระของฝ่ายใดจากข้อมูลนี้</Message>
      </div>

      <div class="diagnosis-section">
        <h3>หลักฐานการส่งคำขอ</h3>
        <dl class="evidence-grid">
          <dt>จำนวนคำขอ</dt><dd>{{ protocolRequestLabel(diagnosis) }}</dd>
          <template v-if="protocol?.requestRef"><dt>Request Ref</dt><dd class="technical-value">{{ protocol.requestRef }}</dd></template>
          <dt>ระยะเวลารวม</dt><dd>{{ formatDurationMs(evidenceDuration) }}</dd>
          <template v-if="protocol?.httpStatus !== undefined"><dt>HTTP Status</dt><dd>{{ protocol.httpStatus }}</dd></template>
          <template v-if="protocol?.responseContentType"><dt>ประเภทข้อมูลตอบกลับ</dt><dd>{{ protocol.responseContentType }}</dd></template>
          <template v-if="protocol?.responseBodyBytes !== undefined"><dt>ขนาดคำตอบ</dt><dd>{{ byteLabel(protocol.responseBodyBytes) }}</dd></template>
          <dt>ข้อมูลเปรียบเทียบ</dt><dd>{{ matchingSuccessLabel(diagnosis) }}</dd>
          <dt>Baseline</dt><dd>{{ diagnosis.baseline.sampleCount.toLocaleString('th-TH') }} งาน<span v-if="diagnosis.baseline.p90Ms !== undefined"> · p90 {{ formatDurationMs(diagnosis.baseline.p90Ms) }}</span></dd>
        </dl>
      </div>

      <slot name="connection" />

      <div class="diagnosis-section">
        <h3>ผลกระทบและสิ่งที่ระบบป้องกัน</h3>
        <p>{{ reportImpactLabel(occurrence.impact) }}</p>
        <Message severity="info" :closable="false" class="m-0">{{ lineImpactLabel(occurrence.impact?.notificationOutcome) }}</Message>
      </div>

      <div class="diagnosis-section admin-action">
        <h3>Admin ควรทำอะไรต่อ</h3>
        <p>{{ diagnosis.assessment.customerActionTh }}</p>
        <p class="text-muted-color mb-0">ผู้รับผิดชอบหลัก: {{ diagnosis.assessment.ownerTh }}</p>
        <Button v-if="diagnosis.customerMessageTh" label="คัดลอกคำตอบให้ลูกค้า" icon="pi pi-copy" outlined class="mt-3" @click="$emit('copyCustomer', diagnosis)" />
      </div>
    </template>
  </section>
</template>

<style scoped>
.diagnosis-panel { min-width: 0; }
.diagnosis-context { display: flex; align-items: flex-start; justify-content: space-between; gap: 1rem; margin-bottom: 1.25rem; }
.diagnosis-context h2 { margin: .2rem 0 .2rem; font-size: 1.25rem; }
.diagnosis-context p { margin: 0; color: var(--text-color-secondary); }
.context-eyebrow { color: var(--text-color-secondary); font-size: .875rem; }
.diagnosis-lead { display: grid; gap: .4rem; padding: 1rem 1.1rem; border-inline-start: .3rem solid var(--p-primary-color); background: color-mix(in srgb, var(--p-primary-color) 7%, var(--surface-card)); border-radius: var(--content-border-radius); }
.diagnosis-lead span, .fact-card span { color: var(--text-color-secondary); font-size: .875rem; }
.diagnosis-lead strong { font-size: 1.15rem; line-height: 1.5; }
.diagnosis-facts { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: .75rem; margin-top: 1rem; }
.fact-card { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: .35rem .65rem; align-content: start; padding: 1rem; border: 1px solid var(--surface-border); border-radius: var(--content-border-radius); }
.fact-card i { grid-row: 1 / 3; color: var(--p-primary-color); margin-top: .15rem; }
.fact-card strong { line-height: 1.45; }
.diagnosis-section { margin-top: 1.25rem; padding-top: 1.25rem; border-top: 1px solid var(--surface-border); }
.diagnosis-section h3 { margin: 0 0 .75rem; font-size: 1rem; }
.diagnosis-section p { line-height: 1.55; }
.evidence-grid { display: grid; grid-template-columns: minmax(10rem, auto) minmax(0, 1fr); gap: .65rem 1.25rem; margin: 0; }
.evidence-grid dt { color: var(--text-color-secondary); }
.evidence-grid dd { margin: 0; overflow-wrap: anywhere; }
.technical-value { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
.admin-action { border-inline-start: .25rem solid var(--p-primary-color); padding-inline-start: 1rem; }
@media (max-width: 860px) { .diagnosis-facts { grid-template-columns: 1fr; } }
@media (max-width: 640px) {
  .diagnosis-context { flex-direction: column; }
  .evidence-grid { grid-template-columns: 1fr; gap: .25rem; }
  .evidence-grid dd { margin-bottom: .65rem; }
}
</style>
