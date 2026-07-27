/** US4 — when the live stream is unavailable, the list still loads and updates
 *  via the existing polling fallback (never a frozen list). */
import { expect, test } from '@playwright/test';
import { login, resetMock, seedTasks, tickMock } from './helpers';

test.beforeEach(async () => {
  await resetMock();
});

async function readPercent(text: string): Promise<number> {
  const m = text.match(/(\d+)% of/);
  return m ? Number(m[1]) : -1;
}

test('falls back to polling when the stream cannot be reached', async ({ page }) => {
  // Force every stream connection to fail; the client must degrade to polling.
  await page.route('**/v1/tasks/stream', (route) => route.abort());

  await seedTasks([
    { name: 'polled.iso', status: 'downloading', size: 1 << 30, downloaded: 0, rate: 25_000_000 },
  ]);
  await login(page);

  const item = page.getByTestId('task-item');
  // The list loads despite the dead stream.
  await expect(item).toContainText('polled.iso');

  const first = await readPercent(await item.innerText());
  await tickMock(20);
  // And it keeps advancing — via the polling fallback, no reload.
  await expect
    .poll(async () => readPercent(await item.innerText()), { timeout: 10_000 })
    .toBeGreaterThan(first + 10);
});
