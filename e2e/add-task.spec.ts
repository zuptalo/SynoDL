/** US3 — Add a download: URLs, torrent upload, destination picker, caps. */
import { expect, test } from '@playwright/test';
import { login, openNewTask, resetMock, seedTasks } from './helpers';

test.beforeEach(async () => {
  await resetMock();
  await seedTasks([]);
});

test('multi-URL input counts links, creates one task per URL with the picked destination', async ({ page }) => {
  await login(page);
  await openNewTask(page);

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
  // Both land in the media folder tv-show/Friends, so the rows are titled after
  // that folder (the readable title) rather than the raw file/magnet name.
  await expect(items.filter({ hasText: 'Friends' })).toHaveCount(2);
});

test('a large mixed-delimiter paste is parsed and added in batches of ten', async ({ page }) => {
  await login(page);
  await openNewTask(page);

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

test('cancel leaves the destination picker without changing the destination', async ({ page }) => {
  await login(page);
  await openNewTask(page);
  await page.getByTestId('newtask-destination').click();
  await expect(page.getByTestId('folder-item').first()).toBeVisible();
  await page.getByTestId('folder-cancel').click();
  await expect(page.getByTestId('newtask-destination')).toContainText('Default folder');
});

test('create a subfolder inside a folder and select it as the destination', async ({ page }) => {
  await login(page);
  await openNewTask(page);
  await page.getByTestId('newtask-destination').click();
  await page.getByTestId('folder-item').filter({ hasText: 'movie' }).click();

  await page.getByTestId('folder-new').click();
  const alert = page.locator('ion-alert');
  await expect(alert).toBeVisible();
  await alert.locator('input').fill('MyPicks');
  await alert.getByRole('button', { name: 'Create' }).click();

  // Wait until the picker has drilled into the freshly created folder (the
  // create is async, and Select is already visible at the parent), then Select.
  await expect(page.getByTestId('folder-title')).toHaveText('/movie/MyPicks');
  await page.getByTestId('folder-confirm').click();
  await expect(page.getByTestId('newtask-destination')).toContainText('movie/MyPicks');
});

test('the destination picker reopens inside the current destination, not root', async ({ page }) => {
  await login(page);
  await openNewTask(page);
  await page.getByTestId('newtask-destination').click();
  await page.getByTestId('folder-item').filter({ hasText: 'movie' }).click();
  await page.getByTestId('folder-confirm').click();
  await expect(page.getByTestId('newtask-destination')).toContainText('movie');

  // Reopen: it must start inside /movie (title shows the path), not at the root.
  await page.getByTestId('newtask-destination').click();
  await expect(page.getByTestId('folder-title')).toHaveText('/movie');
});

test('create a folder in the current destination right from the task screen', async ({ page }) => {
  await login(page);
  await openNewTask(page);
  await page.getByTestId('newtask-destination').click();
  await page.getByTestId('folder-item').filter({ hasText: 'movie' }).click();
  await page.getByTestId('folder-confirm').click();
  await expect(page.getByTestId('newtask-destination')).toContainText('movie');

  await page.getByTestId('newtask-newfolder').click();
  const alert = page.locator('ion-alert');
  await expect(alert).toBeVisible();
  await alert.locator('input').fill('QuickPicks');
  await alert.getByRole('button', { name: 'Create' }).click();
  await expect(page.getByTestId('newtask-destination')).toContainText('movie/QuickPicks');
});

test('the folder picker search filters by a partial name match anywhere', async ({ page }) => {
  await login(page);
  await openNewTask(page);
  await page.getByTestId('newtask-destination').click();
  await expect(page.getByTestId('folder-item').first()).toBeVisible();

  // "vid" matches anywhere in the name → music-video and rated-video.
  await page.getByTestId('folder-search').locator('input').fill('vid');
  const items = page.getByTestId('folder-item');
  await expect(items).toHaveCount(2);
  await expect(items.filter({ hasText: 'music-video' })).toHaveCount(1);
  await expect(items.filter({ hasText: 'rated-video' })).toHaveCount(1);

  // Clearing the search restores the full list.
  await page.getByTestId('folder-search').locator('input').fill('');
  await expect(items.filter({ hasText: 'home' })).toHaveCount(1);
});

test('favoriting a folder gives a one-tap quick-select chip', async ({ page }) => {
  await login(page);
  await openNewTask(page);
  await page.getByTestId('newtask-destination').click();
  await page.getByTestId('folder-item').filter({ hasText: 'movie' }).click();
  await page.getByTestId('folder-favorite').click();
  await page.getByTestId('folder-cancel').click();

  const chip = page.getByTestId('newtask-favorite').filter({ hasText: 'movie' });
  await expect(chip).toBeVisible();
  await chip.click();
  await expect(page.getByTestId('newtask-destination')).toContainText('movie');
});

test('paste appends bulk URLs (never glued into one) and Clear empties the box', async ({
  page,
  context,
}) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await login(page);
  await openNewTask(page);

  // First bulk of 2.
  await page.evaluate(() => navigator.clipboard.writeText('http://a/1.iso\nhttp://a/2.iso'));
  await page.getByTestId('newtask-paste').click();
  await expect(page.getByTestId('newtask-count')).toHaveText('2 links detected');

  // A second paste must APPEND on a fresh line — the 3 new links count as 3,
  // not as one glued token (the reported bug).
  await page.evaluate(() =>
    navigator.clipboard.writeText('http://a/3.iso http://a/4.iso, http://a/5.iso'),
  );
  await page.getByTestId('newtask-paste').click();
  await expect(page.getByTestId('newtask-count')).toHaveText('5 links detected');

  // Clear empties everything (and the Clear button hides when empty).
  await page.getByTestId('newtask-clear').click();
  await expect(page.getByTestId('newtask-count')).toHaveText('0 links detected');
  await expect(page.getByTestId('newtask-clear')).toHaveCount(0);
});

test('junk-only input keeps the confirm button disabled', async ({ page }) => {
  await login(page);
  await openNewTask(page);
  await page.getByTestId('newtask-urls').locator('textarea').fill('definitely not a link');
  await expect(page.getByTestId('newtask-count')).toHaveText('0 links detected');
  // ion-button is a custom element: Playwright's toBeDisabled only understands
  // native form controls, so assert Ionic's aria reflection instead.
  await expect(page.getByTestId('newtask-submit')).toHaveAttribute('aria-disabled', 'true');
});

test('a .torrent upload becomes a task named after the file', async ({ page }) => {
  await login(page);
  await openNewTask(page);
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
  await openNewTask(page);
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
