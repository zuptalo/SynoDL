/**
 * YouTube downloads have their own button (spec 1033).
 *
 * The point of the spec is what the dedicated sheet does NOT contain, so most
 * of these assert absence: no destination picker, no category, no file field,
 * and no mode selector left behind in the general sheet.
 */
import { expect, test } from '@playwright/test';
import { login } from './helpers';

async function gotoTasks(page: import('@playwright/test').Page): Promise<void> {
  await login(page);
  await page.goto('/tabs/tasks');
  await expect(page.getByTestId('newtask-fab')).toBeVisible({ timeout: 30_000 });
}

/** Open the FAB list, which is collapsed until the + is tapped. */
async function openFab(page: import('@playwright/test').Page): Promise<void> {
  await page.getByTestId('newtask-fab').click();
  await expect(page.getByTestId('newtask-open')).toBeVisible();
}

test('the + menu offers a dedicated YouTube action', async ({ page }) => {
  await gotoTasks(page);
  await openFab(page);
  // FR-001, and FR-002 by implication: the e2e stack configures the libraries,
  // so the action must be here.
  await expect(page.getByTestId('ytdl-open')).toBeVisible();
});

test('the YouTube sheet asks for a link and a mode, and nothing else', async ({ page }) => {
  await gotoTasks(page);
  await openFab(page);
  await page.getByTestId('ytdl-open').click();

  await expect(page.getByTestId('ytdl-url')).toBeVisible();
  await expect(page.getByTestId('ytdl-mode')).toBeVisible();

  // FR-003 — the whole reason this sheet exists. None of these apply to a
  // library download, and their presence is what made the old sheet ambiguous.
  await expect(page.getByTestId('newtask-destination')).toHaveCount(0);
  await expect(page.getByTestId('newtask-urls')).toHaveCount(0);
  await expect(page.getByTestId('newtask-file')).toHaveCount(0);
  await expect(page.locator('ion-modal:visible')).not.toContainText('Destination');
  await expect(page.locator('ion-modal:visible')).not.toContainText('Category');
});

test('a mode must be chosen before anything can be sent', async ({ page }) => {
  await gotoTasks(page);
  await openFab(page);
  await page.getByTestId('ytdl-open').click();

  await page.getByTestId('ytdl-url').locator('textarea').fill('https://youtu.be/JDS7zS7_DwE');
  // FR-004: a link alone is not enough — nothing is preselected, so Add stays
  // disabled until the user says which library they mean.
  //
  // ion-button is a custom element, so Playwright's toBeDisabled does not apply
  // (add-task.spec.ts hit the same thing); Ionic exposes the state as
  // aria-disabled, which is also what a screen reader reads.
  await expect(page.getByTestId('ytdl-submit')).toHaveAttribute('aria-disabled', 'true');

  await page.getByTestId('ytdl-mode-music').click();
  // Ionic REMOVES aria-disabled when enabling rather than setting it "false",
  // so assert its absence rather than a value it never takes.
  await expect(page.getByTestId('ytdl-submit')).not.toHaveAttribute('aria-disabled', 'true');
});

test('submitting from the new sheet puts a row in the Tasks list', async ({ page }) => {
  await gotoTasks(page);
  await openFab(page);
  await page.getByTestId('ytdl-open').click();

  await page.getByTestId('ytdl-url').locator('textarea').fill('https://youtu.be/fabtest01');
  await page.getByTestId('ytdl-mode-music').click();
  await page.getByTestId('ytdl-submit').click();

  await expect(page.getByTestId('ytdl-list')).toBeVisible({ timeout: 20_000 });
  await expect(page.getByTestId('ytdl-item').first()).toBeVisible();
});

test('a link that is not YouTube is refused, in words', async ({ page }) => {
  await gotoTasks(page);
  await openFab(page);
  await page.getByTestId('ytdl-open').click();

  await page.getByTestId('ytdl-url').locator('textarea').fill('https://vimeo.com/12345');
  await page.getByTestId('ytdl-mode-music').click();
  await page.getByTestId('ytdl-submit').click();

  // FR-007: say why, rather than failing silently or generically.
  await expect(page.getByTestId('ytdl-error')).toBeVisible({ timeout: 15_000 });
  await expect(page.getByTestId('ytdl-error')).toContainText(/not a supported YouTube address/i);
});

test('the general sheet no longer offers a mode, and points at the button instead', async ({
  page,
}) => {
  await gotoTasks(page);
  await openFab(page);
  await page.getByTestId('newtask-open').click();

  await page
    .getByTestId('newtask-urls')
    .locator('textarea')
    .fill('https://www.youtube.com/watch?v=JDS7zS7_DwE');

  // FR-005: the selector is gone from here entirely.
  await expect(page.getByTestId('ytdl-mode')).toHaveCount(0);
  // FR-006: and the user is told where it should go.
  await expect(page.getByTestId('ytdl-nudge')).toBeVisible();
});

test('a non-YouTube link in the general sheet is untouched by any of this', async ({ page }) => {
  await gotoTasks(page);
  await openFab(page);
  await page.getByTestId('newtask-open').click();

  await page.getByTestId('newtask-urls').locator('textarea').fill('https://example.com/a.torrent');

  // FR-009: no nudge, no mode, and the destination controls still there.
  await expect(page.getByTestId('ytdl-nudge')).toHaveCount(0);
  await expect(page.getByTestId('ytdl-mode')).toHaveCount(0);
  await expect(page.locator('ion-modal:visible')).toContainText('Destination');
});

test('the YouTube sheet can paste a link from the clipboard', async ({ page, context }) => {
  // Spec 1037, FR-007. The general new-task sheet already offers this; the
  // YouTube one is where links actually get pasted.
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await gotoTasks(page);
  await page.evaluate(() => navigator.clipboard.writeText('https://youtu.be/zSGhyrF7YVo'));

  await openFab(page);
  await page.getByTestId('ytdl-open').click();
  await expect(page.getByTestId('ytdl-url')).toBeVisible();

  await page.getByTestId('ytdl-paste').click();

  await expect(page.getByTestId('ytdl-count')).toContainText('1 link detected');
});
