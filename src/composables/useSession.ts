/**
 * Session state for both modes (spec 0003 transition):
 *  - legacy (stateless): the NAS sid lives in IndexedDB and rides X-Syno-Sid.
 *  - stateful: a SynoDL session token lives in IndexedDB and rides
 *    X-SynoDL-Session; the server owns the NAS connection.
 * Mode is detected once at boot via GET /v1/setup/state (a 404 => legacy).
 * IndexedDB remains the client's own store (Principles III + IV); no object
 * store is added, so DB_VERSION is unchanged.
 */
import { computed, ref } from 'vue';
import {
  api,
  setSid,
  setSessionToken,
  SESSION_EXPIRED_EVENT,
  type SetupPayload,
  type SynoDLUser,
} from '@/services/api';
import { del, get, put } from '@/db/idb';

type Mode = 'unknown' | 'legacy' | 'stateful';

interface SessionRow {
  id: 'session';
  mode?: Mode;
  sid?: string;
  account?: string;
  token?: string;
  user?: SynoDLUser;
}

const mode = ref<Mode>('unknown');
const sid = ref('');
const account = ref('');
const token = ref('');
const user = ref<SynoDLUser | null>(null);
const configured = ref(true);
const prefillNasUrl = ref('');
const restored = ref(false);

/** One-shot boot restore: detect mode, then rehydrate any persisted session. */
export async function restoreSession(): Promise<void> {
  if (restored.value) return;
  try {
    const state = await api.setupState();
    mode.value = state.stateful ? 'stateful' : 'legacy';
    configured.value = state.configured;
    prefillNasUrl.value = state.prefillNasUrl ?? '';
  } catch {
    // A network hiccup at boot: assume legacy; signing in still works and will
    // re-detect on the next request.
    mode.value = 'legacy';
  }
  try {
    const row = await get<SessionRow>('settings', 'session');
    if (mode.value === 'stateful' && row?.token) {
      token.value = row.token;
      user.value = row.user ?? null;
      account.value = row.user?.username ?? '';
      setSessionToken(row.token);
    } else if (mode.value === 'legacy' && row?.sid) {
      sid.value = row.sid;
      account.value = row.account ?? '';
      setSid(row.sid);
    }
  } catch {
    // IndexedDB unavailable (private mode) — run signed-out.
  }
  restored.value = true;
}

async function clearLocal(): Promise<void> {
  sid.value = '';
  account.value = '';
  token.value = '';
  user.value = null;
  setSid('');
  setSessionToken('');
  try {
    await del('settings', 'session');
  } catch {
    /* nothing persisted */
  }
}

// A 401 "session" from any request means the current session died — drop it
// once, globally; the router guard then bounces to /login.
if (typeof window !== 'undefined') {
  window.addEventListener(SESSION_EXPIRED_EVENT, () => {
    void clearLocal();
  });
}

async function persist(row: SessionRow): Promise<void> {
  try {
    await put<SessionRow>('settings', row);
  } catch {
    // Private mode: the session just won't survive a reload.
  }
}

async function adoptStateful(res: { token: string; user: SynoDLUser }): Promise<void> {
  token.value = res.token;
  user.value = res.user;
  account.value = res.user.username;
  configured.value = true;
  setSessionToken(res.token);
  await persist({ id: 'session', mode: 'stateful', token: res.token, user: res.user });
}

export function useSession() {
  const isAuthenticated = computed(() =>
    mode.value === 'stateful' ? token.value !== '' : sid.value !== '',
  );
  const needsSetup = computed(() => mode.value === 'stateful' && !configured.value);
  const isAdmin = computed(() => user.value?.isAdmin === true);

  async function login(u: string, password: string, otp?: string): Promise<void> {
    const res = await api.login(u, password, otp);
    sid.value = res.sid;
    account.value = res.account;
    setSid(res.sid);
    await persist({ id: 'session', mode: 'legacy', sid: res.sid, account: res.account });
  }

  async function synodlLogin(username: string, password: string): Promise<void> {
    await adoptStateful(await api.synodlLogin(username, password));
  }

  async function completeSetup(p: SetupPayload): Promise<void> {
    await adoptStateful(await api.submitSetup(p));
  }

  async function logout(): Promise<void> {
    try {
      await api.logout();
    } catch {
      // Best-effort.
    }
    await clearLocal();
  }

  return {
    mode,
    isAuthenticated,
    needsSetup,
    isAdmin,
    account,
    user,
    prefillNasUrl,
    login,
    synodlLogin,
    completeSetup,
    logout,
  };
}
