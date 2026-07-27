/** US2 — See my downloads live: fields, stats, empty state, live updates. */
import { expect, test } from '@playwright/test';
import { login, resetMock, seedTasks } from './helpers';

test.beforeEach(async () => {
  await resetMock();
});

test('renders every task field from seeded fixtures', async ({ page }) => {
  await seedTasks([
    {
      name: 'field-check.iso',
      status: 'downloading',
      size: 2_147_483_648, // 2.0 GB
      downloaded: 1_073_741_824, // 50%
      rate: 0,
      peers: 12,
      seeders: 34,
      destination: 'movie',
    },
  ]);
  // The mock reports a downloading task's speed as its rate (0 here), so the
  // per-task speed shows a dash but percent/size/status are exact.
  await login(page);
  const item = page.getByTestId('task-item').first();
  await expect(item).toContainText('field-check.iso');
  await expect(item.getByTestId('task-status')).toHaveText('downloading');
  await expect(item).toContainText('50% of 2.0 GB');
});

test('header shows NAS-wide speeds from the statistic endpoint', async ({ page }) => {
  await seedTasks([
    { name: 'a', status: 'downloading', size: 1 << 30, rate: 8_500_000 },
    { name: 'b', status: 'downloading', size: 1 << 30, rate: 1_500_000 },
  ]);
  await login(page);
  // 10,000,000 B/s → "9.5 MB/s"
  await expect(page.getByTestId('global-speeds')).toContainText('↓ 9.5 MB/s');
});

test('empty NAS shows a friendly empty state, not an error', async ({ page }) => {
  await seedTasks([]);
  await login(page);
  await expect(page.getByTestId('tasks-empty')).toContainText('No download tasks.');
});

test('a state change on the NAS appears within one polling interval', async ({ page }) => {
  await seedTasks([{ name: 'living.iso', status: 'downloading', size: 1000, downloaded: 100, rate: 0 }]);
  await login(page);
  await expect(page.getByTestId('task-status')).toHaveText('downloading');
  // The NAS-side state changes (re-seeded); the open tab must catch up on its
  // own within the poll interval — no user interaction here.
  await seedTasks([{ name: 'living.iso', status: 'paused', size: 1000, downloaded: 100 }]);
  await expect(page.getByTestId('task-status')).toHaveText('paused', { timeout: 10_000 });
});

test('a zero-size task (magnet resolving metadata) renders 0% safely', async ({ page }) => {
  await seedTasks([{ name: 'magnet-pending', status: 'downloading', size: 0, downloaded: 0, rate: 0 }]);
  await login(page);
  const item = page.getByTestId('task-item').first();
  await expect(item).toContainText('0% of 0 B');
});
