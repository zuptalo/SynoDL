/** US3 — Add a download: URLs, torrent upload, destination picker, caps. */
import { expect, test } from '@playwright/test';
import { login, resetMock, seedTasks } from './helpers';

test.beforeEach(async () => {
  await resetMock();
  await seedTasks([]);
});

test('multi-URL input counts links, creates one task per URL with the picked destination', async ({ page }) => {
  await login(page);
  await page.getByTestId('newtask-open').click();

  await page
    .getByTestId('newtask-urls')
    .locator('textarea')
    .fill('http://mirror.example/one.iso\nnot a url line\nmagnet:?xt=urn:btih:abc123');
  await expect(page.getByTestId('newtask-count')).toHaveText('2 links detected');

  // Destination: drill shares → tv-show → Friends, confirm.
  await page.getByTestId('newtask-destination').click();
  await page.getByTestId('folder-item').filter({ hasText: 'tv-show' }).click();
  await page.getByTestId('folder-item').filter({ hasText: 'Friends' }).click();
  await page.getByTestId('folder-confirm').click();
  await expect(page.getByTestId('newtask-destination')).toContainText('tv-show/Friends');

  await page.getByTestId('newtask-submit').click();
  // Regression (fix/2001): a successful create returns 201 with an empty body;
  // the modal MUST dismiss with no error — not stay open on a false
  // "Could not reach the server." while the task was actually created.
  await expect(page.getByTestId('newtask-submit')).toBeHidden();
  await expect(page.getByTestId('newtask-error')).toHaveCount(0);
  const items = page.getByTestId('task-item');
  await expect(items).toHaveCount(2);
  await expect(items.filter({ hasText: 'one.iso' })).toHaveCount(1);
});

test('a large mixed-delimiter paste is parsed and added in batches of ten', async ({ page }) => {
  await login(page);
  await page.getByTestId('newtask-open').click();

  // 12 links separated by a mix of commas, semicolons, spaces, tabs, newlines.
  const urls = Array.from({ length: 12 }, (_, i) => `http://mirror.example/file${i}.iso`);
  const blob =
    urls.slice(0, 5).join(', ') + '; ' + urls.slice(5, 9).join('\n') + '\t' + urls.slice(9).join(' ');
  await page.getByTestId('newtask-urls').locator('textarea').fill(blob);
  await expect(page.getByTestId('newtask-count')).toHaveText('12 links detected');

  await page.getByTestId('newtask-submit').click();
  // Sent as two batches (10 + 2); all 12 must land with no error.
  await expect(page.getByTestId('newtask-submit')).toBeHidden();
  await expect(page.getByTestId('newtask-error')).toHaveCount(0);
  await expect(page.getByTestId('task-item')).toHaveCount(12);
});

test('junk-only input keeps the confirm button disabled', async ({ page }) => {
  await login(page);
  await page.getByTestId('newtask-open').click();
  await page.getByTestId('newtask-urls').locator('textarea').fill('definitely not a link');
  await expect(page.getByTestId('newtask-count')).toHaveText('0 links detected');
  // ion-button is a custom element: Playwright's toBeDisabled only understands
  // native form controls, so assert Ionic's aria reflection instead.
  await expect(page.getByTestId('newtask-submit')).toHaveAttribute('aria-disabled', 'true');
});

test('a .torrent upload becomes a task named after the file', async ({ page }) => {
  await login(page);
  await page.getByTestId('newtask-open').click();
  await page.getByTestId('newtask-file').setInputFiles({
    name: 'great-distro.torrent',
    mimeType: 'application/x-bittorrent',
    buffer: Buffer.from('d8:announce30:http://tracker.example/announcee'),
  });
  await page.getByTestId('newtask-submit').click();
  await expect(page.getByTestId('newtask-submit')).toBeHidden(); // modal dismissed on success (fix/2001)
  await expect(page.getByTestId('task-item').filter({ hasText: 'great-distro.torrent' })).toHaveCount(1);
});

test('an oversized torrent is refused with a clear message', async ({ page }) => {
  await login(page);
  await page.getByTestId('newtask-open').click();
  // The e2e server runs the default 16 MiB cap; send 17 MiB.
  await page.getByTestId('newtask-file').setInputFiles({
    name: 'huge.torrent',
    mimeType: 'application/x-bittorrent',
    buffer: Buffer.alloc(17 * 1024 * 1024, 120),
  });
  await page.getByTestId('newtask-submit').click();
  await expect(page.getByTestId('newtask-error')).toHaveText(
    'That torrent file is too large to upload.',
  );
});
