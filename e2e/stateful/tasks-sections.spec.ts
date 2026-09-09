/**
 * Every task says where it came from, and an upload says what it is (spec 1042).
 *
 * The Tasks list grew four kinds of row and labelled only two: uploads and
 * YouTube downloads had headings, and then everything from the NAS simply began.
 * An upload row, meanwhile, said less than any other row in the list and could
 * not be opened at all.
 */
import { expect, test, type Page } from '@playwright/test';
import { addSource, apiToken, clearSources, clearYtdl, login, setSourceState } from './helpers';

const K8S = `http://localhost:${process.env.SYNODL_E2E_SF_K8S_PORT || 8296}`;

const API = `http://localhost:${Number(process.env.SYNODL_E2E_SF_PORT) || 8283}`;

let token = '';

test.beforeEach(async () => {
  token = await apiToken();
  // A music upload starts a TAGGING worker (spec 1040), so this spec leaves
  // cluster state behind like the download specs do. Clearing it here — rather
  // than relying on the next spec to — keeps that from being somebody else's
  // intermittent failure.
  await fetch(`${K8S}/__mock/reset`, { method: 'POST' });
  await clearYtdl(token);
  await clearSources(token);
  await setSourceState('reset');
  await addSource(token, 'Only Source', 0);
  await setMusicLibraries('music', 'music-video');
});

async function setMusicLibraries(music: string, musicVideo: string): Promise<void> {
  const res = await fetch(`${API}/v1/library/music`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json', 'X-SynoDL-Session': token },
    body: JSON.stringify({ music, musicVideo }),
  });
  expect(res.status).toBe(200);
}

async function gotoTasks(page: Page): Promise<void> {
  await login(page);
  await page.goto('/tabs/tasks');
  await expect(page.getByTestId('task-list').or(page.getByTestId('tasks-empty'))).toBeVisible({
    timeout: 20_000,
  });
}

/** Send a track through the upload sheet, so the row under test is a real one. */
let takeNo = 0;

async function uploadTrack(page: Page, track: string, artist: string, album: string): Promise<void> {
  // A distinct file name per upload: the same track twice is a collision on the
  // NAS, which is correct behaviour and not what any of these tests is about.
  takeNo += 1;
  await page.getByTestId('newtask-fab').click();
  await page.getByTestId('upload-open').click();
  await page.getByTestId('upload-kind-music').click();
  await page.getByTestId('upload-track').locator('input').fill(track);
  await page.getByTestId('upload-artist').locator('input').fill(artist);
  if (album) await page.getByTestId('upload-album').locator('input').fill(album);
  await page.getByTestId('upload-input').setInputFiles([
    { name: `take-${takeNo}.mp3`, mimeType: 'audio/mpeg', buffer: Buffer.from(`audio ${takeNo}`) },
    // A 1x1 PNG, so the row has real artwork to render from the device.
    {
      name: 'art.png',
      mimeType: 'image/png',
      buffer: Buffer.from(
        'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
        'base64',
      ),
    },
  ]);
  await page.getByTestId('upload-send').click();
}

test('a download sent from Discover sits under a heading that says so', async ({ page }) => {
  await gotoTasks(page);
  // Whatever the fixtures hold, a task with no catalog id is not from Discover
  // and must not be filed under Discover's heading.
  const direct = page.getByTestId('task-list-direct');
  await expect(direct).toBeVisible({ timeout: 20_000 });
  await expect(direct).toContainText('Added by link');
  // And nothing is left unlabelled: every row is inside one of the two lists.
  const inSections = await page
    .locator(
      '[data-testid="task-list-discover"] ion-item-sliding, [data-testid="task-list-direct"] ion-item-sliding',
    )
    .count();
  const allRows = await page.getByTestId('task-item').count();
  expect(inSections).toBe(allRows);
});

test('an empty section is not shown at all', async ({ page }) => {
  await gotoTasks(page);
  // The fixtures have no Discover-sent downloads, so that heading must be absent
  // rather than present and empty.
  await expect(page.getByText('From Discover', { exact: true })).toHaveCount(0);
});

test('an upload row shows its artwork, track, artist and album', async ({ page }) => {
  await gotoTasks(page);
  await uploadTrack(page, 'Lucente', 'Anyma', 'The End Of Genesys');

  const row = page.getByTestId('upload-item').first();
  await expect(row).toBeVisible({ timeout: 20_000 });
  await expect(row.getByTestId('upload-name')).toHaveText('Lucente');
  await expect(row).toContainText('Anyma');
  await expect(row).toContainText('The End Of Genesys');
  await expect(row).toContainText('Music');

  // Rendered from the device: the src is a local object URL, so drawing the row
  // makes no request (FR-010, SC-004).
  const art = page.getByTestId('upload-artwork').first();
  await expect(art).toBeVisible();
  await expect(art).toHaveAttribute('src', /^blob:/);
});

test('the state reads as the same chip every other row uses', async ({ page }) => {
  await gotoTasks(page);
  await uploadTrack(page, 'Sonder', 'Anyma', '');

  const status = page.getByTestId('upload-status').first();
  await expect(status).toBeVisible({ timeout: 20_000 });
  const chip = await status.evaluate((el) => {
    const s = getComputedStyle(el);
    return { background: s.backgroundColor, radius: s.borderRadius };
  });
  expect(chip.background).not.toBe('rgba(0, 0, 0, 0)');
  expect(parseFloat(chip.radius)).toBeGreaterThan(0);
});

test('tapping an upload opens everything known about it', async ({ page }) => {
  await gotoTasks(page);
  await uploadTrack(page, 'Lucente', 'Anyma', 'The End Of Genesys');

  await expect(page.getByTestId('upload-item').first()).toBeVisible({ timeout: 20_000 });
  await page.getByTestId('upload-item').first().click();

  await expect(page.getByTestId('upload-detail')).toBeVisible();
  await expect(page.getByTestId('upload-detail-title')).toHaveText('Lucente');
  await expect(page.getByTestId('upload-detail-artist')).toHaveText('Anyma');
  await expect(page.getByTestId('upload-detail-album')).toHaveText('The End Of Genesys');
  await expect(page.getByTestId('upload-detail-kind')).toHaveText('Music');
  await expect(page.getByTestId('upload-detail-state')).toBeVisible();
  await expect(page.getByTestId('upload-detail-artwork')).toHaveAttribute('src', /^blob:/);
});

test('an upload with no album says Singles, which is where it actually goes', async ({ page }) => {
  await gotoTasks(page);
  await uploadTrack(page, 'Sonder', 'Anyma', '');

  await expect(page.getByTestId('upload-item').first()).toBeVisible({ timeout: 20_000 });
  await page.getByTestId('upload-item').first().click();
  // Saying "—" would describe a different folder from the one the server files
  // it in.
  await expect(page.getByTestId('upload-detail-album')).toHaveText('Singles');
});
