/** US2 (spec 0002) — a failed download shows *why*, not just "error". */
import { expect, test } from '@playwright/test';
import { login, resetMock, seedTasks } from './helpers';

test.beforeEach(async () => {
  await resetMock();
});

test('an errored task shows a human-readable failure reason', async ({ page }) => {
  await seedTasks([
    { name: 'broken.iso', type: 'bt', status: 'error', errorDetail: 'broken_link' },
    { name: 'ok.iso', type: 'http', status: 'finished', size: 100, downloaded: 100 },
  ]);
  await login(page);

  const errored = page.getByTestId('task-item').filter({ hasText: 'broken.iso' });
  await expect(errored.getByTestId('task-error-reason')).toHaveText('Broken link');
  // The healthy task shows its size/progress, not a reason.
  await expect(
    page.getByTestId('task-item').filter({ hasText: 'ok.iso' }).getByTestId('task-error-reason'),
  ).toHaveCount(0);
});

test('an unknown error detail degrades to a generic message', async ({ page }) => {
  await seedTasks([{ name: 'weird.iso', type: 'bt', status: 'error', errorDetail: 'some_new_code' }]);
  await login(page);
  await expect(
    page.getByTestId('task-item').filter({ hasText: 'weird.iso' }).getByTestId('task-error-reason'),
  ).toHaveText('Download failed');
});
