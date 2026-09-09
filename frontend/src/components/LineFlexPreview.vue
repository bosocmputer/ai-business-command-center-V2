<script setup lang="ts">
import { computed } from 'vue';
import type { FlexPreview } from '@/api';
import { formatDateTime } from '@/utils/format';

const props = defineProps<{ preview: FlexPreview }>();
const supportedPresentationVersions = new Set(['ai-bcc-executive-report-v2']);
const payloadSize = computed(() => `${(props.preview.payloadBytes / 1024).toFixed(1)} KB`);
const isExecutiveReport = computed(() => !!props.preview.presentationVersion && supportedPresentationVersions.has(props.preview.presentationVersion));
const isUnsupportedVersion = computed(() => !!props.preview.presentationVersion && !isExecutiveReport.value);
type PreviewReport = FlexPreview['reports'][number];
function primaryFor(report: PreviewReport) { return report.primary ?? report.metrics[1] ?? report.metrics[0]; }
function supportingFor(report: PreviewReport) { return (report.supporting ?? report.metrics.filter((item) => item !== primaryFor(report))).slice(0, 4); }
function isZero(report: PreviewReport) { return report.dataState === 'ZERO' && !!report.stateText; }
function statusFor(report: PreviewReport) {
  if (isZero(report)) return { text: report.stateText ?? '', severity: 'notice' as const };
  if (report.attention?.severity === 'DANGER') return { text: report.attention.text, severity: 'critical' as const };
  if (report.attention?.severity === 'WARNING') return { text: report.attention.text, severity: 'notice' as const };
  return { text: 'พร้อมใช้งาน', severity: 'ready' as const };
}
function insightFor(report: PreviewReport) {
  if (isZero(report)) return report.stateText ?? '';
  if (report.attention?.severity === 'INFO' && !report.comparison) return report.attention.text;
  return 'กดปุ่มด้านล่างเพื่อดูรายละเอียดเพิ่มเติม';
}
function noteFor(report: PreviewReport) {
  if (report.attention && (report.attention.severity === 'WARNING' || report.attention.severity === 'DANGER')) {
    return report.attention.text;
  }
  return '';
}
function generatedAtLabel(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return formatDateTime(value);
  const options = { timeZone: 'Asia/Bangkok' } as const;
  const day = new Intl.DateTimeFormat('th-TH', { ...options, day: 'numeric', month: 'short', year: 'numeric' }).format(date);
  const time = new Intl.DateTimeFormat('th-TH', { ...options, hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(date);
  return `${day} · ${time} น.`;
}
</script>

<template>
  <section class="flex-preview-shell" aria-label="ตัวอย่าง LINE Flex Message">
    <div class="flex-preview-meta">
      <div>
        <span class="font-semibold">ตัวอย่าง Card จาก Backend</span>
        <p class="m-0 mt-1 text-sm text-muted-color">ตัวเลขสมมติเท่านั้น · ไม่ดึงข้อมูลจาก SML</p>
        <p class="m-0 mt-1 text-xs text-muted-color">แบบอักษรอาจต่างเล็กน้อยตาม iOS/Android · แต่ละรายงานคือการ์ดแยกกัน (carousel)</p>
      </div>
      <Tag severity="secondary" :value="payloadSize" />
    </div>

    <Message v-if="isUnsupportedVersion" class="flex-preview-version-warning" severity="warn" :closable="false">
      Preview เวอร์ชัน {{ preview.presentationVersion }} ยังไม่รองรับ ตัวอย่างอาจไม่ตรงกับข้อความจริง
    </Message>

    <p v-if="preview.contextNote" class="flex-preview-context-note">{{ preview.contextNote }}</p>

    <div class="flex-preview-carousel" :data-presentation-version="preview.presentationVersion || 'legacy'">
      <article v-for="report in preview.reports" :key="report.key" class="flex-preview-card">
        <header class="flex-preview-header">
          <span class="flex-preview-kicker">{{ report.categoryLabel }}</span>
          <h3>{{ report.label }}</h3>
          <p class="flex-preview-subtitle">{{ preview.tenantName }} · {{ report.periodLabel || preview.periodLabel }}</p>
        </header>
        <div class="flex-preview-body">
          <div class="flex-preview-status-row">
            <span class="flex-preview-status" :data-severity="statusFor(report).severity">{{ statusFor(report).text }}</span>
            <span class="flex-preview-updated">อัปเดต {{ generatedAtLabel(preview.generatedAt) }}</span>
          </div>
          <strong v-if="!isZero(report) && primaryFor(report)" class="flex-preview-primary-amount">{{ primaryFor(report)?.value }}</strong>
          <div v-for="metric in isZero(report) ? [] : supportingFor(report)" :key="metric.label" class="flex-preview-metric">
            <span>{{ metric.label }}</span>
            <strong>{{ metric.value }}</strong>
          </div>
          <hr class="flex-preview-separator" />
          <div class="flex-preview-insight">
            <span>วันนี้ควรรู้อะไร</span>
            <p>{{ insightFor(report) }}</p>
          </div>
          <div v-if="report.comparison" class="flex-preview-insight">
            <span>เทียบยอด</span>
            <p>{{ report.comparison.text }}</p>
          </div>
          <p v-if="noteFor(report)" class="flex-preview-note" :data-severity="report.attention?.severity">{{ noteFor(report) }}</p>
        </div>
        <footer class="flex-preview-footer">
          <span role="button" aria-disabled="true">เปิดรายละเอียด</span>
        </footer>
      </article>
    </div>
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

.flex-preview-context-note { margin: 0; color: #6b7280; font-size: 0.75rem; line-height: 1.4; }

.flex-preview-carousel {
  display: flex;
  gap: 0.75rem;
  overflow-x: auto;
  padding-bottom: 0.25rem;
  scroll-snap-type: x mandatory;
}

.flex-preview-card {
  flex: 0 0 auto;
  width: min(85vw, 300px);
  scroll-snap-align: start;
  overflow: hidden;
  border: 1px solid #e5e7eb;
  border-radius: 0.75rem;
  background: #fff;
  color: #111827;
  box-shadow: 0 1px 2px rgb(0 0 0 / 0.06);
}

.flex-preview-header { padding: 1rem; background: #f8fafc; }
.flex-preview-kicker { display: block; margin-bottom: 0.2rem; color: #2563eb; font-size: 0.72rem; font-weight: 700; }
.flex-preview-header h3 { display: -webkit-box; overflow: hidden; margin: 0; color: #111827; font-size: 1.05rem; font-weight: 700; overflow-wrap: anywhere; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }
.flex-preview-subtitle { margin: 0.35rem 0 0; color: #6b7280; font-size: 0.82rem; }

.flex-preview-body { padding: 1rem; display: grid; gap: 0.55rem; }
.flex-preview-status-row { display: flex; align-items: baseline; justify-content: space-between; gap: 0.5rem; font-size: 0.72rem; }
.flex-preview-status { font-weight: 700; color: #047857; }
.flex-preview-status[data-severity='notice'] { color: #b45309; }
.flex-preview-status[data-severity='critical'] { color: #b42318; }
.flex-preview-updated { color: #6b7280; }

.flex-preview-primary-amount { font-size: 1.5rem; font-weight: 700; color: #111827; overflow-wrap: anywhere; }

.flex-preview-metric { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 0.5rem; align-items: baseline; font-size: 0.82rem; }
.flex-preview-metric span { min-width: 0; color: #6b7280; overflow-wrap: anywhere; }
.flex-preview-metric strong { min-width: 0; color: #111827; text-align: right; font-variant-numeric: tabular-nums; }

.flex-preview-separator { margin: 0.2rem 0; border: none; border-top: 1px solid #e5e7eb; }

.flex-preview-insight { display: grid; gap: 0.15rem; }
.flex-preview-insight span { color: #6b7280; font-size: 0.68rem; font-weight: 700; }
.flex-preview-insight p { margin: 0; color: #111827; font-size: 0.82rem; overflow-wrap: anywhere; }

.flex-preview-note { margin: 0; padding: 0.5rem 0.6rem; border-radius: 0.4rem; background: #fff7ed; color: #9a3412; font-size: 0.72rem; }

.flex-preview-footer { padding: 0 1rem 1rem; }
.flex-preview-footer span { display: block; padding: 0.6rem; border-radius: 0.45rem; background: #2563eb; color: #fff; text-align: center; font-size: 0.85rem; font-weight: 600; }

@media (max-width: 480px) {
  .flex-preview-shell { padding: 0.75rem; }
  .flex-preview-meta { align-items: stretch; flex-direction: column; }
}
</style>
