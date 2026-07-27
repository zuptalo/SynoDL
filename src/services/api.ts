/**
 * Same-origin HTTP wrapper for the /v1 API. Attaches the NAS session id
 * (X-Syno-Sid) from the in-memory session, converts non-2xx responses into
 * typed ApiError, and broadcasts session expiry so the router can bounce to
 * the login page from one place.
 */
import type { Folder, ServerConfig, Stats, Task } from '@/types/task';

export class ApiError extends Error {
  /** The server's error string: "session", "credentials", "otp_required",
   *  "otp_invalid", "permission", "nas_unreachable", "nas", or free text. */
  readonly code: string;
  readonly status: number;
  constructor(code: string, status: number) {
    super(code);
    this.code = code;
    this.status = status;
  }
}

/** Fires when any request comes back 401 "session" — the sid is dead. */
export const SESSION_EXPIRED_EVENT = 'synodl:session-expired';

/** Fires when the NAS connection needs an admin 2FA re-auth (503 nas_reauth). */
export const NAS_REAUTH_EVENT = 'synodl:nas-reauth';

// Two auth mechanisms coexist during the 0003 transition: the legacy NAS sid
// (X-Syno-Sid, stateless mode) and the SynoDL session token (X-SynoDL-Session,
// stateful mode). Only one is ever set; both headers are attached harmlessly and
// the server reads whichever its mode uses.
let currentSid = '';
let currentToken = '';

/** The api module holds the sid for outbound requests; useSession owns persistence. */
export function setSid(sid: string): void {
  currentSid = sid;
}

/** Stateful mode: the SynoDL session token attached as X-SynoDL-Session. */
export function setSessionToken(token: string): void {
  currentToken = token;
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (currentSid) headers.set('X-Syno-Sid', currentSid);
  if (currentToken) headers.set('X-SynoDL-Session', currentToken);
  const resp = await fetch(path, { ...init, headers });
  if (!resp.ok) {
    let code = `http_${resp.status}`;
    try {
      const body = (await resp.json()) as { error?: string };
      if (body.error) code = body.error;
    } catch {
      /* non-JSON error body — keep the status code */
    }
    if (resp.status === 401 && code === 'session' && (currentSid || currentToken)) {
      window.dispatchEvent(new CustomEvent(SESSION_EXPIRED_EVENT));
    }
    if (resp.status === 503 && code === 'nas_reauth') {
      window.dispatchEvent(new CustomEvent(NAS_REAUTH_EVENT));
    }
    throw new ApiError(code, resp.status);
  }
  // Some successful responses carry no body: 204 from pause/resume/delete, and
  // 201 Created from task-create. Parsing an empty body as JSON throws, so read
  // the text once and only parse when there is something to parse — otherwise a
  // successfully created task surfaced as a false "Could not reach the server."
  const text = await resp.text();
  return (text ? (JSON.parse(text) as T) : (undefined as T));
}

function json(body: unknown): RequestInit {
  return {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  };
}

function jsonMethod(method: string, body: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  };
}

/** A SynoDL account as the admin sees it (stateful mode). */
export interface AdminUser {
  id: number;
  username: string;
  isAdmin: boolean;
  isEnabled: boolean;
}

/** A SynoDL account (stateful mode). */
export interface SynoDLUser {
  id: number;
  username: string;
  isAdmin: boolean;
}

/** A user's notification preferences (spec 1004). */
export interface NotifPrefs {
  notifyAdded: boolean;
  notifyCompleted: boolean;
  notifyFailed: boolean;
  scope: 'own' | 'any';
}

/** The non-secret NAS connection projection (no password) from GET /v1/nas/config. */
export interface NasConfig {
  publicUrl: string;
  nasAddress: string;
  nasPort: number;
  nasTlsVerify: boolean;
  nasAccount: string;
  nasUses2FA: boolean;
}

/** Fields for testing/updating the NAS connection. A blank password keeps the stored one. */
export interface NasConnInput {
  publicUrl?: string;
  nasAddress?: string;
  nasPort?: number;
  nasTlsVerify?: boolean;
  nasAccount?: string;
  nasPassword?: string;
  otp?: string;
}

/** Whether the instance is stateful and, if so, whether setup has run. */
export interface SetupState {
  stateful: boolean;
  configured: boolean;
  prefillNasUrl?: string;
}

/** Fields the first-run wizard collects. */
export interface SetupPayload {
  publicUrl: string;
  nasAddress: string;
  nasPort: number;
  nasTlsVerify: boolean;
  nasAccount: string;
  nasPassword: string;
  otp?: string;
  adminUsername: string;
  adminPassword: string;
}

export interface TaskSnapshot {
  tasks: Task[];
  stats: Stats;
}

/** One parsed SSE frame: a named event and/or a JSON data payload. */
function parseSSEFrame(frame: string): { event?: string; data?: unknown } | null {
  let event: string | undefined;
  const dataLines: string[] = [];
  for (const line of frame.split('\n')) {
    if (line === '' || line.startsWith(':')) continue; // heartbeat / comment
    if (line.startsWith('event:')) event = line.slice(6).trim();
    else if (line.startsWith('data:')) dataLines.push(line.slice(5).replace(/^ /, ''));
  }
  if (dataLines.length === 0 && event === undefined) return null;
  let data: unknown;
  if (dataLines.length) {
    try {
      data = JSON.parse(dataLines.join('\n'));
    } catch {
      return null;
    }
  }
  return { event, data };
}

/**
 * Consume the live task stream (GET /v1/tasks/stream) via fetch + a
 * ReadableStream reader — NOT EventSource — so the session rides in a header,
 * never the URL (constitution Principle III). `onSnapshot` fires for each
 * snapshot; the promise resolves when the caller aborts via `signal` or the
 * server closes cleanly, and rejects with an ApiError otherwise:
 *
 *  - code 'session' / status 401: connect-time 401 or a terminal session_expired
 *    event — the caller must NOT fall back; the session-expiry flow takes over.
 *  - any other error: a transport failure — the caller falls back to polling and
 *    retries the stream with backoff.
 */
export async function streamTasks(
  onSnapshot: (snap: TaskSnapshot) => void,
  signal: AbortSignal,
): Promise<void> {
  const headers = new Headers({ Accept: 'text/event-stream' });
  if (currentSid) headers.set('X-Syno-Sid', currentSid);
  if (currentToken) headers.set('X-SynoDL-Session', currentToken);

  let resp: Response;
  try {
    resp = await fetch('/v1/tasks/stream', { headers, signal });
  } catch {
    if (signal.aborted) return; // caller stopped us — a clean shutdown
    throw new ApiError('nas_unreachable', 0);
  }

  if (!resp.ok || !resp.body) {
    let code = `http_${resp.status}`;
    try {
      const body = (await resp.json()) as { error?: string };
      if (body.error) code = body.error;
    } catch {
      /* non-JSON body — keep the status code */
    }
    if (resp.status === 401 && code === 'session') {
      window.dispatchEvent(new CustomEvent(SESSION_EXPIRED_EVENT));
    }
    if (resp.status === 503 && code === 'nas_reauth') {
      window.dispatchEvent(new CustomEvent(NAS_REAUTH_EVENT));
    }
    throw new ApiError(code, resp.status);
  }

  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buf = '';
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) return; // server closed the stream
      buf += decoder.decode(value, { stream: true });
      let sep: number;
      // SSE frames are separated by a blank line.
      while ((sep = buf.indexOf('\n\n')) !== -1) {
        const frame = buf.slice(0, sep);
        buf = buf.slice(sep + 2);
        const evt = parseSSEFrame(frame);
        if (!evt) continue; // heartbeat / unparseable
        if (evt.event === 'error') {
          // Terminal auth error: surface it exactly like a 401 on a poll.
          window.dispatchEvent(new CustomEvent(SESSION_EXPIRED_EVENT));
          throw new ApiError('session', 401);
        }
        if (evt.data) onSnapshot(evt.data as TaskSnapshot);
      }
    }
  } catch (e) {
    if (signal.aborted) return; // caller aborted mid-read — clean
    throw e instanceof ApiError ? e : new ApiError('nas', 0);
  } finally {
    void reader.cancel().catch(() => undefined);
  }
}

export const api = {
  config: () => request<ServerConfig>('/v1/config'),

  // Legacy stateless mode: authenticate to the NAS, carry the sid.
  login: (account: string, password: string, otp?: string) =>
    request<{ sid: string; account: string }>('/v1/session', json({ account, password, otp })),

  logout: () => request<void>('/v1/session', { method: 'DELETE' }),

  // Stateful mode (spec 0003): setup wizard + SynoDL accounts.
  // setupState probes /v1/setup/state; a 404 means the server is in legacy mode.
  setupState: async (): Promise<SetupState> => {
    try {
      const s = await request<{ configured: boolean; prefillNasUrl?: string }>('/v1/setup/state');
      return { stateful: true, configured: s.configured, prefillNasUrl: s.prefillNasUrl };
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) return { stateful: false, configured: false };
      throw e;
    }
  },
  submitSetup: (p: SetupPayload) =>
    request<{ token: string; user: SynoDLUser }>('/v1/setup', json(p)),
  synodlLogin: (username: string, password: string) =>
    request<{ token: string; user: SynoDLUser }>('/v1/session', json({ username, password })),
  me: () => request<SynoDLUser>('/v1/me'),
  nasReauth: (otp: string) => request<void>('/v1/nas/reauth', json({ otp })),

  // Per-user notification preferences (spec 1004).
  getNotifPrefs: () => request<NotifPrefs>('/v1/notifications/prefs'),
  setNotifPrefs: (p: NotifPrefs) => request<void>('/v1/notifications/prefs', jsonMethod('PUT', p)),

  // Admin: view/edit the stored NAS connection and test it before saving
  // (spec 1002). The password is write-only — the server never returns it.
  getNasConfig: () => request<NasConfig>('/v1/nas/config'),
  testNasConnection: (input: NasConnInput) => request<void>('/v1/nas/test', json(input)),
  updateNasConfig: (input: NasConnInput) => request<void>('/v1/nas/config', jsonMethod('PUT', input)),

  tasks: () => request<{ tasks: Task[]; stats: Stats }>('/v1/tasks'),

  createTaskURIs: (
    uris: string[],
    opts: { destination?: string; username?: string; password?: string; unzipPassword?: string } = {},
  ) => request<void>('/v1/tasks', json({ uris, ...opts })),

  createTaskFile: (file: File, opts: { destination?: string; unzipPassword?: string } = {}) => {
    const form = new FormData();
    form.set('torrent', file, file.name);
    if (opts.destination) form.set('destination', opts.destination);
    if (opts.unzipPassword) form.set('unzipPassword', opts.unzipPassword);
    return request<void>('/v1/tasks', { method: 'POST', body: form });
  },

  pauseTasks: (ids: string[]) => request<void>('/v1/tasks/pause', json({ ids })),
  resumeTasks: (ids: string[]) => request<void>('/v1/tasks/resume', json({ ids })),
  deleteTasks: (ids: string[]) => request<void>('/v1/tasks/delete', json({ ids })),

  shares: () => request<{ folders: Folder[] }>('/v1/fs/shares'),
  listFolder: (path: string) =>
    request<{ folders: Folder[] }>(`/v1/fs/list?path=${encodeURIComponent(path)}`),
  // Create a subfolder `name` under the absolute parent `path` (spec 1006).
  createFolder: (path: string, name: string) =>
    request<{ folder: Folder }>('/v1/fs/folder', json({ path, name })),

  // Admin user management + per-user folder grants (stateful mode, Increment 3).
  listUsers: () => request<{ users: AdminUser[] }>('/v1/users'),
  createUser: (username: string, password: string, isAdmin: boolean) =>
    request<AdminUser>('/v1/users', json({ username, password, isAdmin })),
  updateUser: (id: number, patch: { isEnabled?: boolean; isAdmin?: boolean; password?: string }) =>
    request<AdminUser>(`/v1/users/${id}`, jsonMethod('PATCH', patch)),
  deleteUser: (id: number) => request<void>(`/v1/users/${id}`, { method: 'DELETE' }),
  getUserFolders: (id: number) => request<{ folders: string[] }>(`/v1/users/${id}/folders`),
  setUserFolders: (id: number, folders: string[]) =>
    request<{ folders: string[] }>(`/v1/users/${id}/folders`, jsonMethod('PUT', { folders })),

  // Web Push (stateful mode, Increment 4).
  pushKey: () => request<{ publicKey: string }>('/v1/push/key'),
  saveSubscription: (endpoint: string, keys: { p256dh: string; auth: string }, optedIn: boolean) =>
    request<void>('/v1/push/subscription', json({ endpoint, keys, optedIn })),
  deleteSubscription: (endpoint: string) =>
    request<void>('/v1/push/subscription', jsonMethod('DELETE', { endpoint })),
};
