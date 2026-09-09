---
status: current
last_verified: 2026-07-21
source_of_truth: [src/main.ts, src/constants/themeConstants.ts, src/layout/AppShell.vue, src/components/dashboard/ExecutiveChart.vue, src/components/operations/IncidentDiagnosisPanel.vue]
tags: [frontend, ui, accessibility, performance]
---

# UI Quality Gates

## Visual System

- Use PrimeVue/Sakai composition and shared theme tokens.
- Default primary palette is PrimeVue blue; preserve semantic success, warning, and danger colors.
- Fonts are self-hosted Lato and Noto Sans Thai.
- Support light and dark modes without component-specific hard-coded theme replacements.
- Main content uses the Sakai card rhythm; avoid introducing a parallel radius/shadow system.

## Responsive and Accessibility

- Support 320px and wider without page-level horizontal overflow.
- Important touch targets are at least 44×44px.
- Shared `AppShell` owns topbar, mobile drawer, focus trap, Escape handling, body scroll lock, and cleanup.
- Mobile header uses contextual tenant/report information; desktop retains the Sakai shell.
- Forms expose labels and server errors; dialogs trap focus and restore it when closed.
- Data visualizations retain an accessible table or semantic list fallback.
- Do not communicate profit/loss, warning, or failure through color alone.

## Data Presentation

- UI dates use `th-TH` and `Asia/Bangkok`, labelled as Thai time where ambiguity exists.
- API values are not mutated for display; identifiers retain leading zeros.
- Numeric table headers/cells are right-aligned with tabular numerals.
- Missing, zero, partial, stale, and invalid data are distinct states.
- Operational diagnosis uses a progressive hierarchy: plain-language cause,
  affected subsystem, investigation owner, JavaWS status, request evidence,
  containment, then the next action. Hashes and raw transport fields remain out
  of the primary Admin surface.
- Narrow ranking/composition charts use semantic lists; trend charts retain bounded ticks and reduced motion.

## Performance Budgets

- Largest JavaScript chunk: at most 75 KB gzip
- Main CSS: at most 12 KB gzip
- INP: at most 200 ms
- CLS: at most 0.1
- Route and chart chunks remain lazy; polling must not remount stable chart/table instances.
- Admin P1 badge polling pauses while the document is hidden, aborts stale requests, and cleans timers/listeners on unmount.

## Required QA

- Unit and component tests for changed behavior
- `typecheck`, contract verification, lint, unit tests, and production build
- Browser checks at 390px and 1440px, including keyboard, dark mode, loading/empty/error/session-expired states
- Console and network inspection for unexpected requests, especially SML-triggering viewer calls
