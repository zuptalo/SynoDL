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
};
