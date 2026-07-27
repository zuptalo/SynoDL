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

let currentSid = '';

/** The api module holds the sid for outbound requests; useSession owns persistence. */
export function setSid(sid: string): void {
  currentSid = sid;
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (currentSid) headers.set('X-Syno-Sid', currentSid);
  const resp = await fetch(path, { ...init, headers });
  if (!resp.ok) {
    let code = `http_${resp.status}`;
    try {
      const body = (await resp.json()) as { error?: string };
      if (body.error) code = body.error;
    } catch {
      /* non-JSON error body — keep the status code */
    }
    if (resp.status === 401 && code === 'session' && currentSid) {
      window.dispatchEvent(new CustomEvent(SESSION_EXPIRED_EVENT));
    }
    throw new ApiError(code, resp.status);
  }
  if (resp.status === 204) return undefined as T;
  return (await resp.json()) as T;
}

function json(body: unknown): RequestInit {
  return {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  };
}

export const api = {
  config: () => request<ServerConfig>('/v1/config'),

  login: (account: string, password: string, otp?: string) =>
    request<{ sid: string; account: string }>('/v1/session', json({ account, password, otp })),

  logout: () => request<void>('/v1/session', { method: 'DELETE' }),

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
};
