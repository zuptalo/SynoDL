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
  kind: 'zarfilm' | 'thirtynama' = 'zarfilm',
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
