/**
 * Helpers for the stateful e2e specs (spec 0007).
 *
 * These run against the second stack global-setup brings up: a stateful synodl
 * on :8283 with a TLS mock DSM on :8294, fronted by a vite on :5275. Its
 * first-run setup has already been done, so a spec signs in with the SynoDL
 * account rather than NAS credentials.
 */
import { expect, type Page } from '@playwright/test';

const SF_PORT = Number(process.env.SYNODL_E2E_SF_PORT) || 8283;
const SF_MOCK_PORT = Number(process.env.SYNODL_E2E_SF_MOCK_PORT) || 8294;

const API = `http://localhost:${SF_PORT}`;
const MOCK = `https://localhost:${SF_MOCK_PORT}`;

export const ADMIN = { username: 'e2eadmin', password: 'e2e-admin-password' };

/** The mock speaks TLS with a per-run self-signed certificate, like a NAS. */
async function mockFetch(path: string, init?: RequestInit): Promise<Response> {
  const prev = process.env.NODE_TLS_REJECT_UNAUTHORIZED;
  process.env.NODE_TLS_REJECT_UNAUTHORIZED = '0';
  try {
    return await fetch(`${MOCK}${path}`, init);
  } finally {
    if (prev === undefined) delete process.env.NODE_TLS_REJECT_UNAUTHORIZED;
    else process.env.NODE_TLS_REJECT_UNAUTHORIZED = prev;
  }
}

/** Drive a fake source's state, e.g. 'zar/logged-out' or 'reset'. */
export async function setSourceState(action: string): Promise<void> {
  const res = await mockFetch(`/__mock/source/${action}`, { method: 'POST' });
  if (!res.ok) throw new Error(`source control ${action} failed: ${res.status}`);
}

/** Sign in through the API and return the session token. */
export async function apiToken(): Promise<string> {
  const res = await fetch(`${API}/v1/session`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(ADMIN),
  });
  if (!res.ok) throw new Error(`login failed: ${res.status} ${await res.text()}`);
  return ((await res.json()) as { token: string }).token;
}

async function api(token: string, path: string, init?: RequestInit): Promise<Response> {
  return fetch(`${API}${path}`, {
    ...init,
    headers: { 'Content-Type': 'application/json', 'X-SynoDL-Session': token, ...(init?.headers ?? {}) },
  });
}

/** Remove every configured source, so each spec starts from a known state. */
export async function clearSources(token: string): Promise<void> {
  const res = await api(token, '/v1/source/providers');
  const { providers } = (await res.json()) as { providers: Array<{ id: number }> };
  for (const p of providers) {
    await api(token, `/v1/source/providers/${p.id}`, { method: 'DELETE' });
  }
  // Also reset the user's saved view, which remembers the selected source.
  await api(token, '/v1/source/view', {
    method: 'PUT',
    body: JSON.stringify({ filters: {}, sort: '', order: '', selectedSource: '' }),
  });
}

/**
 * Configure a source against the fake site. No real credentials exist anywhere
 * in this suite — the fake accepts any non-empty value.
 */
export async function addSource(
  token: string,
  displayName: string,
  sortOrder: number,
  kind: 'zarfilm' | '30nama' = 'zarfilm',
): Promise<number> {
  const res = await api(token, '/v1/source/providers', {
    method: 'POST',
    body: JSON.stringify({
      kind,
      displayName,
      moviesParent: 'movie',
      tvParent: 'tv-show',
      sortOrder,
      // Both drivers' declared fields, so either kind can be created from here.
      // None of these is a real credential — the fakes accept anything.
      session: {
        wordpress_logged_in: 'e2e-not-a-real-cookie',
        cf_clearance: 'e2e-not-a-real-clearance',
        c_token: 'e2e-not-a-real-token',
        c_api_key: 'e2e-not-a-real-key',
        user_agent: 'e2e',
      },
    }),
  });
  if (res.status !== 201) throw new Error(`add source failed: ${res.status} ${await res.text()}`);
  return ((await res.json()) as { id: number }).id;
}

/** Sign in through the UI and land on Discover. */
export async function login(page: Page): Promise<void> {
  await page.addInitScript(() => {
    try {
      window.localStorage.setItem('landing.page', 'browser');
    } catch {
      /* ignore */
    }
  });
  await page.goto('/');
  await expect(page).toHaveURL(/\/login/);
  await page.getByTestId('login-account').locator('input').fill(ADMIN.username);
  await page.getByTestId('login-password').locator('input').fill(ADMIN.password);
  await page.getByTestId('login-submit').click();
  await expect(page).toHaveURL(/\/tabs\//);
}

/** Open Discover and wait for its grid (or its empty state) to settle. */
export async function gotoDiscover(page: Page): Promise<void> {
  await page.goto('/tabs/browser');
  await expect(page.locator('.card, .state').first()).toBeVisible({ timeout: 30_000 });
}

/**
 * Seed folders into the mock NAS's tree, so a spec can set up "this title is
 * already downloaded" (spec 0008). Parents are created implicitly, and seeding
 * is additive — the fixture folders stay.
 */
/**
 * Seed FILES per directory. Ownership reads what a folder CONTAINS, so seeding a
 * folder name alone no longer marks a title — that was the bug (FR-001a).
 * Defaults to reset:false so it composes with a seedLibrary() call that made the
 * folders.
 */
export async function seedLibraryFiles(
  tree: Record<string, string[]>,
  reset = false,
): Promise<void> {
  const res = await mockFetch('/__mock/library', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ reset, tree }),
  });
  if (!res.ok) throw new Error(`seed library files failed: ${res.status}`);
}

export async function seedLibrary(
  folders: Record<string, string[]>,
  reset = true,
): Promise<void> {
  const res = await mockFetch('/__mock/library', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ reset, folders }),
  });
  if (!res.ok) throw new Error(`seed library failed: ${res.status}`);
}

/**
 * Force the ownership snapshot to rebuild.
 *
 * The server holds its reading of the NAS for five minutes (FR-010), so a spec
 * that seeds folders AFTER a search has already warmed the snapshot would
 * otherwise be asserting against a stale one. Touching a source is the same
 * invalidation a real configuration change triggers (FR-008a); a real user gets
 * the same effect by sending a download, or by waiting out the TTL.
 */
export async function refreshLibrary(token: string, sourceId: number, displayName: string): Promise<void> {
  const res = await api(token, `/v1/source/providers/${sourceId}`, {
    method: 'PUT',
    body: JSON.stringify({
      kind: 'zarfilm',
      displayName,
      moviesParent: 'movie',
      tvParent: 'tv-show',
      session: {},
    }),
  });
  if (!res.ok) throw new Error(`refresh library failed: ${res.status} ${await res.text()}`);
}

/** One page of catalog results, straight from the API. */
export interface TitleDetailShape {
  id: string;
  title: string;
  type: string;
  ownership?: string;
  seasons?: Array<{ season: number; episodes: number[]; videoFiles: number }>;
  /** Set by the driver from the title's own page, for sources whose listings
   *  carry neither (spec 1023). */
  imdbId?: string;
  plot?: string;
  qualities?: QualityShape[];
}

export interface QualityShape {
  id: string;
  label: string;
  season?: string;
  resolution?: string;
  encoder?: string;
  /** THIS release is the one on the NAS — not merely that its season is (spec 1025). */
  owned?: boolean;
}

/** Season detail rides on the title endpoint, which already resolves through the
 *  caller's own source access — see contracts/library-api.md §2. */
export async function apiTitle(token: string, id: string): Promise<TitleDetailShape> {
  const res = await api(token, `/v1/source/title/${encodeURIComponent(id)}`);
  if (!res.ok) throw new Error(`title failed: ${res.status} ${await res.text()}`);
  return (await res.json()) as TitleDetailShape;
}

/** Store the hide-owned preference the way the filter sheet does. */
export async function setHideOwned(token: string, hideOwned: boolean): Promise<void> {
  const view = await api(token, '/v1/source/view');
  const cur = (await view.json()) as Record<string, unknown>;
  const res = await api(token, '/v1/source/view', {
    method: 'PUT',
    body: JSON.stringify({ ...cur, hideOwned }),
  });
  if (!res.ok) throw new Error(`set hide-owned failed: ${res.status}`);
}

export interface SearchItem {
  id: string;
  title: string;
  type: string;
  ownership?: string;
  genres?: string[];
  imdbScore?: number;
  sourceName?: string;
}

export async function apiSearch(
  token: string,
  body: Record<string, unknown> = { page: 1 },
): Promise<SearchItem[]> {
  const res = await api(token, '/v1/source/search', { method: 'POST', body: JSON.stringify(body) });
  if (!res.ok) throw new Error(`search failed: ${res.status} ${await res.text()}`);
  return ((await res.json()) as { items: SearchItem[] }).items;
}

export interface Facet {
  value: string;
  name?: string;
  slug?: string;
}

/** The filter facets the sheet would offer — for every source, or just one. */
export async function apiParameters(token: string, source = ''): Promise<Record<string, Facet[]>> {
  const q = source ? `?source=${encodeURIComponent(source)}` : '';
  const res = await api(token, `/v1/source/parameters${q}`);
  if (!res.ok) throw new Error(`parameters failed: ${res.status} ${await res.text()}`);
  return (await res.json()) as Record<string, Facet[]>;
}

/** The slugs of one facet group, which is how two sources' options are compared. */
export function slugs(facets: Facet[] = []): string[] {
  return facets.map((f) => f.slug || '').filter(Boolean);
}

/**
 * The folder name SynoDL would create for a title — a port of sanitizeFolderName
 * in server/internal/api/source_handlers.go. Kept here so a spec can seed the
 * exact folder a real send would have produced.
 */
export function folderNameFor(title: string): string {
  return title
    .replace(/[/\\:*?"<>|\x00-\x1f]/g, ' ')
    .split(/\s+/)
    .filter(Boolean)
    .join(' ')
    .replace(/^[ .]+|[ .]+$/g, '')
    .slice(0, 120)
    .trim();
}
