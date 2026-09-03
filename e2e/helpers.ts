/**
 * Shared e2e helpers: mock-DSM seeding/reset and the login flow. Every spec
 * starts from a deterministic mock state via resetMock()/seedTasks().
 */
import { expect, type Page } from '@playwright/test';

const MOCK = `http://localhost:${process.env.SYNODL_E2E_MOCK_PORT || 8292}`;

/** A /__mock/seed task fixture. Rate 0 freezes progress for stable asserts. */
export interface TaskFixture {
  name: string;
  type?: string;
  status: string;
  size?: number;
  errorDetail?: string;
  downloaded?: number;
  uploaded?: number;
  rate?: number;
  upRate?: number;
  peers?: number;
  seeders?: number;
  createdAt?: number;
  destination?: string;
  uri?: string;
}

/** Restore the mock's default fixtures and drop all sessions. */
export async function resetMock(): Promise<void> {
  const res = await fetch(`${MOCK}/__mock/reset`, { method: 'POST' });
  if (!res.ok) throw new Error(`mock reset failed: ${res.status}`);
}

/** Replace the mock's task list with exactly these fixtures. */
export async function seedTasks(tasks: TaskFixture[]): Promise<void> {
  const res = await fetch(`${MOCK}/__mock/seed`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ tasks }),
  });
  if (!res.ok) throw new Error(`mock seed failed: ${res.status}`);
}

/** Advance the mock's virtual clock (progress for rate>0 downloading tasks). */
export async function tickMock(seconds: number): Promise<void> {
  const res = await fetch(`${MOCK}/__mock/tick`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ seconds }),
  });
  if (!res.ok) throw new Error(`mock tick failed: ${res.status}`);
}

/**
 * Drive the real login screen. Sessions live in IndexedDB, so a fresh
 * Playwright context is always signed out; specs call this once up front.
 */
export async function login(page: Page, account = 'admin', password = 'secret'): Promise<void> {
  // The landing tab is a per-device preference (default: Discover). These specs
  // exercise Tasks, so pin it to Tasks for a deterministic post-login URL.
  await page.addInitScript(() => {
    try {
      window.localStorage.setItem('landing.page', 'tasks');
    } catch {
      /* ignore */
    }
  });
  await page.goto('/');
  await expect(page).toHaveURL(/\/login/);
  await page.getByTestId('login-account').locator('input').fill(account);
  await page.getByTestId('login-password').locator('input').fill(password);
  await page.getByTestId('login-submit').click();
  await expect(page).toHaveURL(/\/tabs\/tasks/);
}

/**
 * Open the "add a task" modal from the Tasks tab.
 *
 * The FAB expands into two ways to put something in the library — fetch it by
 * URL, or upload a file from this device (spec 1022) — so opening the modal is
 * two taps rather than one. It lives here so the interaction is described in a
 * single place instead of at every call site.
 */
export async function openNewTask(page: Page): Promise<void> {
  await page.getByTestId('newtask-fab').click();
  await page.getByTestId('newtask-open').click();
}
