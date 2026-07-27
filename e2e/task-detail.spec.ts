/** US3 — tapping a task row opens a stock-Ionic detail sheet with the full
 *  field set, including the failure reason for an errored task. */
import { expect, test } from '@playwright/test';
import { login, resetMock, seedTasks } from './helpers';

test.beforeEach(async () => {
  await resetMock();
});

test('tapping a row opens the detail sheet with full fields', async ({ page }) => {
  await seedTasks([
    {
      name: 'detail-me.iso',
      status: 'downloading',
      size: 1 << 30,
      downloaded: 1 << 29, // 50%
      rate: 0,
      peers: 5,
      seeders: 9,
      destination: 'home/Downloads',
    },
  ]);
  await login(page);

  await page.getByTestId('task-item').click();
  await expect(page.getByTestId('task-detail')).toBeVisible();
  await expect(page.getByTestId('detail-name')).toHaveText('detail-me.iso');
  await expect(page.getByTestId('detail-status')).toHaveText('downloading');
  await expect(page.getByTestId('detail-destination')).toContainText('home/Downloads');
  await expect(page.getByTestId('detail-progress')).toContainText('50%');

  await page.getByTestId('detail-close').click();
  await expect(page.getByTestId('task-detail')).toBeHidden();
});

test('detail sheet explains why an errored task failed', async ({ page }) => {
  await seedTasks([
    { name: 'broken.iso', status: 'error', size: 1 << 30, downloaded: 0, errorDetail: 'broken_link' },
  ]);
  await login(page);

  await page.getByTestId('task-item').click();
  await expect(page.getByTestId('detail-reason')).toBeVisible();
  await expect(page.getByTestId('detail-reason')).toHaveText('Broken link');
});
