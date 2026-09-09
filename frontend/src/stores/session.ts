import { reactive, readonly } from 'vue';
import { adminApi, type AdminSession } from '@/api';
import { ApiError } from '@/api/client';
import { clearAdminReportCatalog } from '@/stores/reportCatalog';

type SessionStatus = 'unknown' | 'loading' | 'authenticated' | 'anonymous';

const state = reactive<{ status: SessionStatus; session: AdminSession | null }>({ status: 'unknown', session: null });
let pending: Promise<boolean> | null = null;

async function ensureAdminSession(force = false): Promise<boolean> {
  if (!force && state.status === 'authenticated') return true;
  if (!force && state.status === 'anonymous') return false;
  if (pending) return pending;
  state.status = 'loading';
  pending = adminApi.session()
    .then((session) => {
      state.session = session;
      state.status = 'authenticated';
      return true;
    })
    .catch((error: unknown) => {
      if (error instanceof ApiError && error.status === 401) {
        state.session = null;
        state.status = 'anonymous';
        return false;
      }
      state.status = 'unknown';
      throw error;
    })
    .finally(() => { pending = null; });
  return pending;
}

async function login(username: string, password: string): Promise<AdminSession> {
  const session = await adminApi.login(username, password);
  state.session = session;
  state.status = 'authenticated';
  return session;
}

async function logout(): Promise<void> {
  try { await adminApi.logout(); } finally {
    clearAdminReportCatalog();
    state.session = null;
    state.status = 'anonymous';
  }
}

function updateSession(session: AdminSession): void {
  state.session = session;
  state.status = 'authenticated';
}

function clearSession(): void {
  clearAdminReportCatalog();
  state.session = null;
  state.status = 'anonymous';
  pending = null;
}

export function useAdminSession() {
  return { state: readonly(state), ensureAdminSession, login, logout, updateSession, clearSession };
}
