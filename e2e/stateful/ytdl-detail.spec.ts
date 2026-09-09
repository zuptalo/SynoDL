/**
 * Opening one YouTube download (spec 0013, US4).
 *
 * The sheet is where every fact this feature gathers becomes visible, and where
 * the fixed date format is enforced: `2026-11-21` and `14:32:07`, on every
 * device, regardless of the viewer's locale (FR-031). That is a product
 * decision, so it is asserted by shape rather than by comparing against a
 * locale-formatted string, which would just re-implement the bug.
 */
import { expect, test, type Page } from '@playwright/test';
import { apiToken, clearYtdl, login } from './helpers';

const SF_PORT = Number(process.env.SYNODL_E2E_SF_PORT) || 8283;
const K8S = `http://localhost:${process.env.SYNODL_E2E_SF_K8S_PORT || 8296}`;
const API = `http://localhost:${SF_PORT}`;

const DATE_TIME = /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/;

async function submit(token: string, url: string, mode = 'music'): Promise<string> {
  const res = await fetch(`${API}/v1/ytdl`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-SynoDL-Session': token },
    body: JSON.stringify({ url, mode }),
  });
  expect(res.status).toBe(202);
  return ((await res.json()) as { requestId: string }).requestId;
}

/**
 * Drive one job to a lifecycle state, addressed by its request id.
 *
 * Retries on 404, because a submitted download is QUEUED first and its worker
 * does not exist until the reconciler admits it (spec 0013). Waiting for the
 * job to appear is part of driving it.
 */
async function drive(requestId: string, action: string): Promise<void> {
  const deadline = Date.now() + 25_000;
  for (;;) {
    const res = await fetch(`${K8S}/__mock/jobs/${requestId}/${action}`, { method: 'POST' });
    if (res.ok) return;
    if (res.status !== 404 || Date.now() > deadline) {
      throw new Error(`drive ${action} failed: ${res.status}`);
    }
    await new Promise((r) => setTimeout(r, 500));
  }
}

async function gotoTasks(page: Page): Promise<void> {
  await login(page);
  await page.goto('/tabs/tasks');
  await expect(page.getByTestId('task-list').or(page.getByTestId('tasks-empty'))).toBeVisible();
}

let token = '';

test.beforeEach(async () => {
  token = await apiToken();
  await fetch(`${K8S}/__mock/reset`, { method: 'POST' });
  await clearYtdl(token);
});

test('tapping a download opens a sheet with everything known about it', async ({ page }) => {
  const requestId = await submit(token, 'https://youtu.be/zSGhyrF7YVo');
  await drive(requestId, 'start');
  await drive(requestId, 'succeed');

  await gotoTasks(page);
  await page.getByTestId('ytdl-item').first().click();

  const sheet = page.getByTestId('ytdl-detail');
  await expect(sheet).toBeVisible();
  await expect(page.getByTestId('ytdl-detail-state')).toHaveText('saved');
  await expect(page.getByTestId('ytdl-detail-mode')).toHaveText('Music');
  await expect(page.getByTestId('ytdl-detail-scope')).toHaveText('One video');
  await expect(page.getByTestId('ytdl-detail-url')).toContainText('zSGhyrF7YVo');
  await expect(page.getByTestId('ytdl-detail-lyrics')).toBeVisible();
});

test('both timestamps read as 2026-11-21 14:32:07, whatever the device locale', async ({
  page,
}) => {
  const requestId = await submit(token, 'https://youtu.be/zSGhyrF7YVo');
  await drive(requestId, 'start');
  await drive(requestId, 'succeed');

  await gotoTasks(page);
  await page.getByTestId('ytdl-item').first().click();
  await expect(page.getByTestId('ytdl-detail')).toBeVisible();

  await expect(page.getByTestId('ytdl-detail-created')).toHaveText(DATE_TIME);
  await expect(page.getByTestId('ytdl-detail-finished')).toHaveText(DATE_TIME);
});

test('a download that has not finished shows no finish time, not 1970', async ({ page }) => {
  // FR-033. An absent fact is shown as absent; a zero timestamp would render as
  // a date in 1970, which is a confident wrong answer.
  const requestId = await submit(token, 'https://youtu.be/zSGhyrF7YVo');
  await drive(requestId, 'start');

  await gotoTasks(page);
  await page.getByTestId('ytdl-item').first().click();
  await expect(page.getByTestId('ytdl-detail')).toBeVisible();

  await expect(page.getByTestId('ytdl-detail-created')).toHaveText(DATE_TIME);
  await expect(page.getByTestId('ytdl-detail-finished')).toHaveText('—');
});

test('a failed download explains itself in plain language', async ({ page }) => {
  // FR-032: never a command line, a path, or raw worker output.
  const requestId = await submit(token, 'https://youtu.be/zSGhyrF7YVo');
  await drive(requestId, 'start');
  await drive(requestId, 'fail');

  await gotoTasks(page);
  await page.getByTestId('ytdl-item').first().click();
  await expect(page.getByTestId('ytdl-detail')).toBeVisible();
  await expect(page.getByTestId('ytdl-detail-state')).toHaveText('failed');

  const reason = await page.getByTestId('ytdl-detail-reason').innerText();
  expect(reason.trim()).not.toBe('');
  for (const forbidden of ['yt-dlp', '/out', 'exec', 'Traceback', '--']) {
    expect(reason, `reason must not leak ${forbidden}`).not.toContain(forbidden);
  }
});

test('tapping the link copies it', async ({ page, context }) => {
  // Spec 1037, FR-005. Same gesture a NAS task's source link already offers.
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  const requestId = await submit(token, 'https://youtu.be/zSGhyrF7YVo');
  await drive(requestId, 'start');
  await drive(requestId, 'succeed');

  await gotoTasks(page);
  await page.getByTestId('ytdl-item').first().click();
  await expect(page.getByTestId('ytdl-detail')).toBeVisible();

  await page.getByTestId('ytdl-detail-url-row').click();

  const copied = await page.evaluate(() => navigator.clipboard.readText());
  expect(copied).toContain('zSGhyrF7YVo');
});
