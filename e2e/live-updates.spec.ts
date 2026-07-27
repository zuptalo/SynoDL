/** US1 — the task list updates live via the SSE stream, no manual refresh. */
import { expect, test } from '@playwright/test';
import { login, resetMock, seedTasks, tickMock } from './helpers';

test.beforeEach(async () => {
  await resetMock();
});

/** Read the integer percent shown on the (single) task row. */
async function readPercent(text: string): Promise<number> {
  const m = text.match(/(\d+)% of/);
  return m ? Number(m[1]) : -1;
}

test('progress advances live without a page reload', async ({ page }) => {
  await seedTasks([
    { name: 'live.iso', status: 'downloading', size: 1 << 30, downloaded: 0, rate: 25_000_000 },
  ]);
  await login(page);

  const item = page.getByTestId('task-item');
  await expect(item).toContainText('live.iso');

  const first = await readPercent(await item.innerText());
  // Jump the mock's virtual clock; the SSE stream must reflect it on its own.
  await tickMock(20);
  await expect
    .poll(async () => readPercent(await item.innerText()), { timeout: 8000 })
    .toBeGreaterThan(first + 10);

  // Sanity: the advance happened without any navigation.
  await expect(page).toHaveURL(/\/tabs\/tasks/);
});
