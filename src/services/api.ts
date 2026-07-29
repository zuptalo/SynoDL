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

/** Fires on every request with whether the SERVER was reachable (detail.reachable):
 *  false when fetch itself rejects (network down), true when it responds at all. */
export const CONNECTIVITY_EVENT = 'synodl:connectivity';

function reportReachable(reachable: boolean): void {
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new CustomEvent(CONNECTIVITY_EVENT, { detail: { reachable } }));
  }
}

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
  let resp: Response;
  try {
    resp = await fetch(path, { ...init, headers });
  } catch (e) {
    // fetch only rejects on a network-level failure (server unreachable) — an
    // HTTP error still resolves. Surface it as a connectivity signal.
    reportReachable(false);
    throw e;
  }
  reportReachable(true); // the server responded (even a 4xx/5xx means reachable)
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
  /** True for the instance owner — the first account. Protected from other admins. */
  isOwner?: boolean;
  /** Content-rating cap for the catalog ("" = unrestricted, e.g. "G", "PG-13"). */
  contentRating?: string;
  /** Rolling-24h download-count limit (0 = unlimited). */
  dailyDownloadLimit?: number;
  /** Downloads started in the last 24h (for the admin to see who's near their cap). */
  downloadsUsed?: number;
}

/** A SynoDL account (stateful mode). */
export interface SynoDLUser {
  id: number;
  username: string;
  isAdmin: boolean;
}

/** A user's destination preferences (spec 1011): default folder + favorites. */
export interface DestinationPrefs {
  default: string;
  favorites: string[];
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

// Download-source catalog (spec 0005). Session material is write-only: the
// server never returns it, so there is no "get session" — only status.
export interface SourceStatus {
  configured: boolean;
  enabled: boolean;
  state: 'not_configured' | 'active' | 'needs_refresh';
  providerName: string;
  kind: string;
  moviesParent: string;
  tvParent: string;
  /** Instance-wide max download size in MB (0 = unlimited). */
  maxDownloadMB: number;
  lastVerifiedAt: number;
  canManage: boolean;
}
export interface SourceSessionInput {
  kind: string;
  displayName?: string;
  moviesParent: string;
  tvParent?: string;
  session: {
    cfClearance: string;
    cApiKey: string;
    cToken: string;
    userAgent: string;
    cPlatform?: string;
    cAppVersion?: string;
  };
}
export interface CatalogTitle {
  id: string;
  type: string;
  title: string;
  posterUrl: string;
  /** A reliable secondary poster to try if posterUrl fails to load (may be absent). */
  posterFallbackUrl?: string;
  /** The wide cover image, shown large behind the detail header (may be absent). */
  backdropUrl?: string;
  imdbId: string;
  imdbScore: number;
  providerScore: number;
  plot: string;
  genres: string[];
  comingSoon: boolean;
  freeDownload: boolean;
}
export interface SourceSearchResult {
  page: number;
  pages: number;
  items: CatalogTitle[];
}
/** One selectable value in a filter facet, from the provider's parameters. */
export interface SourceFacet {
  value: string;
  name?: string;
  slug?: string;
}
/** The provider's live filter facets (GET /v1/source/parameters). */
export interface SourceParameters {
  genres: SourceFacet[];
  types: SourceFacet[];
  qualities: SourceFacet[];
  scores: SourceFacet[];
  languages: SourceFacet[];
  countries: SourceFacet[];
  minYear: number;
  maxYear: number;
}
export interface QualityOption {
  id: string;
  label: string;
  size: string;
  resolution: string;
  encoder: string;
  hardsub: boolean;
  /** For series: the season this pack covers, and its episode count. */
  season?: string;
  episodes?: number;
}
export interface TitleDetail {
  id: string;
  type: string;
  title: string;
  sendable: boolean;
  qualities: QualityOption[];
}
/** Route a provider cover URL through the same-origin, server-cached image proxy
 *  (empty when there's no poster). */
export function posterSrc(url: string): string {
  return url ? `/v1/source/image?u=${encodeURIComponent(url)}` : '';
}

export interface SourceSearchFilters {
  type?: string;
  quality?: string;
  language?: string;
  country?: string;
  score?: string;
  genre?: string[];
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

  // Per-user destination preferences: default folder + favorites (spec 1011).
  // The server returns the cleaned set (invalid/gone folders removed).
  getDestinationPrefs: () => request<DestinationPrefs>('/v1/destinations/prefs'),
  setDestinationPrefs: (p: DestinationPrefs) =>
    request<DestinationPrefs>('/v1/destinations/prefs', jsonMethod('PUT', p)),

  // Download-source catalog (spec 0005). Off until an admin configures a provider.
  getSourceStatus: () => request<SourceStatus>('/v1/source/status'),
  putSourceSession: (input: SourceSessionInput) =>
    request<{ state: string; lastVerifiedAt: number }>('/v1/source/session', jsonMethod('PUT', input)),
  deleteSourceSession: () => request<{ state: string }>('/v1/source/session', { method: 'DELETE' }),
  setSourcePolicy: (maxDownloadMB: number) =>
    request<{ maxDownloadMB: number }>('/v1/source/policy', jsonMethod('PUT', { maxDownloadMB })),
  searchSource: (query: string, filters: SourceSearchFilters, page: number, sort: string, order: string) =>
    request<SourceSearchResult>('/v1/source/search', json({ query, filters, page, sort, order })),
  getSourceTitle: (id: string) =>
    request<TitleDetail>(`/v1/source/title/${encodeURIComponent(id)}`),
  // episodes (1-based) narrow a series to specific episodes; omit for a movie or
  // the whole season.
  sendSource: (titleId: string, qualityId: string, title: string, type: string, episodes?: number[]) =>
    request<{ destination: string; count: number }>(
      '/v1/source/send',
      json({ titleId, qualityId, title, type, episodes }),
    ),
  // The provider's live filter facets (genres, types, qualities, …) so the filter
  // UI stays in step with the source. Refreshed on open/foreground.
  getSourceParameters: () => request<SourceParameters>('/v1/source/parameters'),
  // The signed-in user's daily download allowance (limit 0 = unlimited, remaining -1).
  getSourceQuota: () => request<{ limit: number; used: number; remaining: number }>('/v1/source/quota'),
  getSourcePrefs: () => request<{ preferredQuality: string }>('/v1/source/prefs'),
  setSourcePrefs: (preferredQuality: string) =>
    request<{ preferredQuality: string }>('/v1/source/prefs', jsonMethod('PUT', { preferredQuality })),
  // Per-user Discover view (facet filters + sort field + direction), synced across devices.
  getSourceView: () => request<{ filters: SourceSearchFilters; sort: string; order: string }>('/v1/source/view'),
  setSourceView: (filters: SourceSearchFilters, sort: string, order: string) =>
    request<void>('/v1/source/view', jsonMethod('PUT', { filters, sort, order })),

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
  updateUser: (
    id: number,
    patch: {
      isEnabled?: boolean;
      isAdmin?: boolean;
      password?: string;
      contentRating?: string;
      dailyDownloadLimit?: number;
    },
  ) =>
    request<AdminUser>(`/v1/users/${id}`, jsonMethod('PATCH', patch)),
  deleteUser: (id: number) => request<void>(`/v1/users/${id}`, { method: 'DELETE' }),
  getUserFolders: (id: number) => request<{ folders: string[] }>(`/v1/users/${id}/folders`),
  setUserFolders: (id: number, folders: string[]) =>
    request<{ folders: string[] }>(`/v1/users/${id}/folders`, jsonMethod('PUT', { folders })),
  // Admin: clear a user's daily download count (fresh allowance now).
  resetUserDownloads: (id: number) =>
    request<void>(`/v1/users/${id}/downloads/reset`, { method: 'POST' }),

  // Web Push (stateful mode, Increment 4).
  pushKey: () => request<{ publicKey: string }>('/v1/push/key'),
  saveSubscription: (endpoint: string, keys: { p256dh: string; auth: string }, optedIn: boolean) =>
    request<void>('/v1/push/subscription', json({ endpoint, keys, optedIn })),
  deleteSubscription: (endpoint: string) =>
    request<void>('/v1/push/subscription', jsonMethod('DELETE', { endpoint })),
};
