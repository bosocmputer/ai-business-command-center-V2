<script setup lang="ts">
import { computed } from 'vue';
import type { FlexPreview } from '@/api';
import { formatDateTime } from '@/utils/format';

const props = defineProps<{ preview: FlexPreview }>();
const supportedPresentationVersions = new Set(['executive-navy-v1', 'executive-navy-v2']);
const payloadSize = computed(() => `${(props.preview.payloadBytes / 1024).toFixed(1)} KB`);
const isExecutiveNavy = computed(() => !!props.preview.presentationVersion && supportedPresentationVersions.has(props.preview.presentationVersion));
const isUnsupportedVersion = computed(() => !!props.preview.presentationVersion && !isExecutiveNavy.value);
type PreviewReport = FlexPreview['reports'][number];
function primaryFor(report: PreviewReport) { return report.primary ?? report.metrics[1] ?? report.metrics[0]; }
function supportingFor(report: PreviewReport) { return report.supporting ?? report.metrics.filter((item) => item !== primaryFor(report)); }
function isZero(report: PreviewReport) { return isExecutiveNavy.value && report.dataState === 'ZERO' && !!report.stateText; }
function showCategory(index: number) { return !!props.preview.reports[index]?.categoryLabel && props.preview.reports[index]?.categoryLabel !== props.preview.reports[index - 1]?.categoryLabel; }
function generatedAtLabel(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return formatDateTime(value);
  const options = { timeZone: 'Asia/Bangkok' } as const;
  const day = new Intl.DateTimeFormat('th-TH', { ...options, day: 'numeric', month: 'short', year: 'numeric' }).format(date);
  const time = new Intl.DateTimeFormat('th-TH', { ...options, hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(date);
  return `${day} · ${time} น. เวลาไทย`;
}
</script>

<template>
  <section class="flex-preview-shell" aria-label="ตัวอย่าง LINE Flex Message">
    <div class="flex-preview-meta">
      <div>
        <span class="font-semibold">ตัวอย่าง Card จาก Backend</span>
        <p class="m-0 mt-1 text-sm text-muted-color">ตัวเลขสมมติเท่านั้น · ไม่ดึงข้อมูลจาก SML</p>
        <p class="m-0 mt-1 text-xs text-muted-color">แบบอักษรอาจต่างเล็กน้อยตาม iOS/Android</p>
      </div>
      <Tag severity="secondary" :value="payloadSize" />
    </div>

    <Message v-if="isUnsupportedVersion" class="flex-preview-version-warning" severity="warn" :closable="false">
      Preview เวอร์ชัน {{ preview.presentationVersion }} ยังไม่รองรับ ตัวอย่างอาจไม่ตรงกับข้อความจริง
    </Message>

    <article
      class="flex-preview-card"
      :class="isExecutiveNavy ? 'is-executive-navy' : 'is-legacy'"
      :data-presentation-version="preview.presentationVersion || 'legacy'"
    >
      <header class="flex-preview-header"><strong>NEXTSTEP DASHBOARD</strong><span>สรุปผู้บริหาร</span></header>
      <div class="flex-preview-body">
        <h3>{{ preview.tenantName }}</h3>
        <p class="flex-preview-period">{{ preview.periodLabel }}</p>
        <p class="flex-preview-count">{{ preview.reports.length }} รายงาน</p>
        <p v-if="preview.contextNote" class="flex-preview-context-note">{{ preview.contextNote }}</p>
        <template v-for="(report, index) in preview.reports" :key="report.key">
          <span v-if="showCategory(index)" class="flex-preview-category">{{ report.categoryLabel }}</span>
          <section class="flex-preview-report">
            <h4>{{ report.label }} <i class="pi pi-angle-right" /></h4>
            <p v-if="preview.mixedPeriods && report.periodLabel" class="flex-preview-report-period">{{ report.periodLabel }}</p>
            <p v-if="isZero(report)" class="flex-preview-state">{{ report.stateText }}</p>
            <div v-else-if="primaryFor(report)" class="flex-preview-metric primary">
              <span>{{ primaryFor(report)?.label }}</span>
              <strong>{{ primaryFor(report)?.value }}</strong>
            </div>
            <p v-if="report.comparison" class="flex-preview-comparison">{{ report.comparison.text }}</p>
            <div v-for="metric in isZero(report) ? [] : supportingFor(report)" :key="metric.label" class="flex-preview-metric">
              <span>{{ metric.label }}</span>
              <strong>{{ metric.value }}</strong>
            </div>
            <p v-if="report.attention" class="flex-preview-attention" :data-severity="report.attention.severity">{{ report.attention.text }}</p>
          </section>
        </template>
        <p class="flex-preview-generated">สร้างเมื่อ {{ generatedAtLabel(preview.generatedAt) }}</p>
      </div>
      <footer class="flex-preview-footer">
        <span role="button" aria-disabled="true">ดูภาพรวมร้าน</span>
      </footer>
    </article>
    <p class="m-0 text-xs text-muted-color safe-wrap">Alt text: {{ preview.altText }}</p>
  </section>
</template>

<style scoped>
.flex-preview-shell {
  display: grid;
  gap: 0.75rem;
  padding: 1rem;
  border: 1px solid var(--surface-border);
  border-radius: 0.875rem;
  background: var(--surface-50);
}

.flex-preview-meta {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
}

.flex-preview-card {
  width: min(100%, 420px);
  margin-inline: auto;
  overflow: hidden;
  border: 1px solid #e3ebf5;
  border-radius: 0.5rem;
  background: #fff;
  color: #0f172a;
}

.flex-preview-header {
  display: grid;
  gap: 0.2rem;
  padding: 1rem;
  background: #0b2347;
  color: #fff;
  font-size: 0.78rem;
  font-weight: 700;
  letter-spacing: 0.04em;
}
.flex-preview-header span { color: #d6e4ff; font-size: 0.72rem; }

.flex-preview-body { padding: 1.125rem; }
.flex-preview-body h3 { display: -webkit-box; overflow: hidden; margin: 0; font-size: 1.25rem; overflow-wrap: anywhere; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }
.flex-preview-period { margin: 0.35rem 0 0; color: #5b6b82; font-size: 0.875rem; }
.flex-preview-count { margin: 0.25rem 0 0; color: #94a3b8; font-size: 0.72rem; }
.flex-preview-context-note { margin: 0.45rem 0 0; color: #5b6b82; font-size: 0.72rem; line-height: 1.4; }

.flex-preview-report {
  margin-top: 0.75rem;
  padding: 0.75rem;
  border: 1px solid #e3ebf5;
  border-radius: 0.5rem;
  background: #f5f8fc;
  container-type: inline-size;
}

.flex-preview-category { display: block; margin: 0.8rem 0 -0.3rem; color: #5b6b82; font-size: 0.68rem; font-weight: 700; }
.flex-preview-report h4 { display: flex; justify-content: space-between; gap: 0.5rem; margin: 0 0 0.55rem; color: #123b6d; font-size: 0.9rem; line-height: 1.35; }
.flex-preview-report h4 i { flex: 0 0 auto; color: #94a3b8; }
.flex-preview-report-period { margin: -0.25rem 0 0.5rem; color: #5b6b82; font-size: 0.68rem; }
.flex-preview-metric { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 0.5rem; align-items: baseline; margin-top: 0.35rem; font-size: 0.78rem; }
.flex-preview-metric.primary { grid-template-columns: minmax(0, 40%) minmax(0, 60%); }
.flex-preview-metric span { min-width: 0; color: #5b6b82; overflow-wrap: anywhere; }
.flex-preview-metric strong { min-width: 0; color: #0f172a; text-align: right; white-space: nowrap; font-size: clamp(0.625rem, 3.5cqi, 0.875rem); font-variant-numeric: tabular-nums; }
.flex-preview-metric.primary strong { font-size: clamp(0.625rem, 4cqi, 1rem); }
.flex-preview-comparison { margin: 0.25rem 0 0; color: #5b6b82; font-size: 0.72rem; }
.flex-preview-state { margin: 0.4rem 0 0; color: #5b6b82; font-size: 0.78rem; }
.flex-preview-attention { margin: 0.55rem 0 0; padding: 0.45rem 0.55rem; border-radius: 0.4rem; background: #e2e8f0; color: #475569; font-size: 0.72rem; font-weight: 700; }
.flex-preview-attention[data-severity="WARNING"] { background: #fef3c7; color: #92400e; }
.flex-preview-attention[data-severity="DANGER"] { background: #fee2e2; color: #b91c1c; }
.flex-preview-generated { margin: 1rem 0 0; color: #94a3b8; font-size: 0.72rem; }
.flex-preview-footer { padding: 0 1rem 1rem; }
.flex-preview-footer span { display: block; padding: 0.65rem; border-radius: 0.45rem; background: #175cd3; color: #fff; text-align: center; font-size: 0.875rem; font-weight: 600; }

.flex-preview-card.is-legacy .flex-preview-header,
.flex-preview-card.is-legacy .flex-preview-footer span { background: #1d4ed8; }
.flex-preview-card.is-legacy .flex-preview-report h4 { color: #1d4ed8; }

@media (max-width: 480px) {
  .flex-preview-shell { padding: 0.75rem; }
  .flex-preview-meta { align-items: stretch; flex-direction: column; }
}
</style>
