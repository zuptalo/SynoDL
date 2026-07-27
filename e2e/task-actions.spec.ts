/** US4 — Control tasks: pause, resume, delete-with-confirmation. */
import { expect, test, type Page } from '@playwright/test';
import { login, resetMock, seedTasks } from './helpers';

test.beforeEach(async () => {
  await resetMock();
});

/** Reveal the sliding options of the first task row (drag-free, deterministic). */
async function openSliding(page: Page): Promise<void> {
  await page.evaluate(() => {
    const sliding = document.querySelector('ion-item-sliding') as HTMLElement & {
      open: (side: string) => Promise<void>;
    };
    return sliding.open('end');
  });
}

test('pause flips a downloading task to paused on the NAS and in the list', async ({ page }) => {
  await seedTasks([{ name: 'pausable.iso', status: 'downloading', size: 1000, downloaded: 10, rate: 0 }]);
  await login(page);
  await expect(page.getByTestId('task-status')).toHaveText('downloading');
  await openSliding(page);
  await page.getByTestId('task-pause').click();
  await expect(page.getByTestId('task-status')).toHaveText('paused', { timeout: 10_000 });
});

test('resume brings a paused task back to downloading', async ({ page }) => {
  await seedTasks([{ name: 'resumable.iso', status: 'paused', size: 1000, downloaded: 10 }]);
  await login(page);
  await expect(page.getByTestId('task-status')).toHaveText('paused');
  await openSliding(page);
  await page.getByTestId('task-resume').click();
  await expect(page.getByTestId('task-status')).toHaveText('downloading', { timeout: 10_000 });
});

test('delete requires confirmation; cancel keeps the task, confirm removes it', async ({ page }) => {
  await seedTasks([{ name: 'doomed.iso', status: 'paused', size: 1000, downloaded: 10 }]);
  await login(page);

  await openSliding(page);
  await page.getByTestId('task-delete').click();
  await page.getByRole('button', { name: 'Cancel' }).click();
  await expect(page.getByTestId('task-item')).toHaveCount(1);

  await openSliding(page);
  await page.getByTestId('task-delete').click();
  // The cancelled sheet's DOM can still be tearing down — target the newest.
  // The confirm button names the count ("Delete 1") per spec 0004.
  await page.getByRole('button', { name: 'Delete 1', exact: true }).last().click();
  await expect(page.getByTestId('tasks-empty')).toBeVisible({ timeout: 10_000 });
});
