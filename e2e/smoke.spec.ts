/**
 * Walking-skeleton smoke: the whole stack — vite → synodl proxy → mock DSM —
 * boots, gates on login, and renders live task data end to end.
 */
import { expect, test } from '@playwright/test';
import { login, resetMock, seedTasks } from './helpers';

test.beforeEach(async () => {
  await resetMock();
});

test('signed-out visitors land on the login screen', async ({ page }) => {
  await page.goto('/');
  await expect(page).toHaveURL(/\/login/);
  await expect(page.getByTestId('login-submit')).toBeVisible();
  // The pre-login config call surfaces the NAS host.
  await expect(page.getByTestId('login-host')).toHaveText('localhost');
});

test('login shows the seeded task list with live fields', async ({ page }) => {
  await seedTasks([
    {
      name: 'e2e-fixture.iso',
      status: 'downloading',
      size: 1_073_741_824,
      downloaded: 268_435_456,
      rate: 0, // frozen: assertions below stay exact
      peers: 7,
      seeders: 21,
      destination: 'home/Downloads',
    },
    { name: 'done.iso', status: 'finished', size: 1000, downloaded: 1000 },
  ]);

  await login(page);
  const items = page.getByTestId('task-item');
  await expect(items).toHaveCount(2);
  // Locate the downloading task by name (the default sort is newest-first, and
  // 'done.iso' was seeded later, so position is not a stable handle).
  const fixture = items.filter({ hasText: 'e2e-fixture.iso' });
  await expect(fixture).toHaveCount(1);
  await expect(fixture).toContainText('25% of 1.0 GB');
  await expect(fixture.getByTestId('task-status')).toHaveText('downloading');

  // Reload keeps the session (IndexedDB persistence) — no login round-trip.
  await page.reload();
  await expect(page).toHaveURL(/\/tabs\/tasks/);
  await expect(page.getByTestId('task-item')).toHaveCount(2);
});

test('logout returns to login and forgets the session', async ({ page }) => {
  await login(page);
  await page.getByTestId('tab-settings').click();
  await expect(page.getByTestId('settings-account')).toHaveText('admin');
  await page.getByTestId('settings-logout').click();
  await expect(page).toHaveURL(/\/login/);
  // A reload must not resurrect the session.
  await page.reload();
  await expect(page).toHaveURL(/\/login/);
});
