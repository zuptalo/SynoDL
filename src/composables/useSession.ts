/**
 * NAS session state. The sid lives in IndexedDB (constitution Principles III +
 * IV: the client owns the session, the server stores nothing) and is mirrored
 * into module state so the router guard and api wrapper read it synchronously.
 */
import { computed, ref } from 'vue';
import { api, setSid, SESSION_EXPIRED_EVENT } from '@/services/api';
import { del, get, put } from '@/db/idb';

interface SessionRow {
  id: 'session';
  sid: string;
  account: string;
}

const sid = ref('');
const account = ref('');
const restored = ref(false);

/** One-shot boot restore: pull the persisted session before the router runs. */
export async function restoreSession(): Promise<void> {
  if (restored.value) return;
  try {
    const row = await get<SessionRow>('settings', 'session');
    if (row?.sid) {
      sid.value = row.sid;
      account.value = row.account;
      setSid(row.sid);
    }
  } catch {
    // IndexedDB unavailable (private mode) — run signed-out; login still works.
  }
  restored.value = true;
}

async function clearLocal(): Promise<void> {
  sid.value = '';
  account.value = '';
  setSid('');
  try {
    await del('settings', 'session');
  } catch {
    /* nothing persisted */
  }
}

// A 401 "session" from ANY request means the NAS revoked/expired the sid —
// drop it once, globally; the router guard then bounces to /login.
if (typeof window !== 'undefined') {
  window.addEventListener(SESSION_EXPIRED_EVENT, () => {
    void clearLocal();
  });
}

export function useSession() {
  const isAuthenticated = computed(() => sid.value !== '');

  async function login(user: string, password: string, otp?: string): Promise<void> {
    const res = await api.login(user, password, otp);
    sid.value = res.sid;
    account.value = res.account;
    setSid(res.sid);
    try {
      await put<SessionRow>('settings', { id: 'session', sid: res.sid, account: res.account });
    } catch {
      // Private mode: the session just won't survive a reload.
    }
  }

  async function logout(): Promise<void> {
    try {
      await api.logout();
    } catch {
      // Best-effort: the NAS session dies on its own timeout if unreachable.
    }
    await clearLocal();
  }

  return { isAuthenticated, account, login, logout };
}
