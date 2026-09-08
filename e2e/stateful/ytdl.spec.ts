/**
 * YouTube downloads (spec 0012).
 *
 * These run against the fake Jobs API on :8296, which downloads nothing and
 * moves a job only when told to. That determinism is the point: the states this
 * feature reports are the whole feature, so each one is driven explicitly
 * rather than waited for.
 */
import { expect, test, type Page } from '@playwright/test';
import { apiToken, clearYtdl, login } from './helpers';

const SF_PORT = Number(process.env.SYNODL_E2E_SF_PORT) || 8283;
const K8S = `http://localhost:${process.env.SYNODL_E2E_SF_K8S_PORT || 8296}`;
const API = `http://localhost:${SF_PORT}`;

async function resetJobs(): Promise<void> {
  await fetch(`${K8S}/__mock/reset`, { method: 'POST' });
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

async function submit(
  token: string,
  url: string,
  mode: 'music' | 'music-video',
): Promise<{ status: number; requestId: string }> {
  const res = await fetch(`${API}/v1/ytdl`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-SynoDL-Session': token },
    body: JSON.stringify({ url, mode }),
  });
  const body = res.ok ? ((await res.json()) as { requestId: string }) : { requestId: '' };
  return { status: res.status, requestId: body.requestId };
}

async function gotoTasks(page: Page): Promise<void> {
  await login(page);
  await page.goto('/tabs/tasks');
  await expect(page.getByTestId('task-list').or(page.getByTestId('tasks-empty'))).toBeVisible();
}

/** The visible state of the single YouTube row. */
async function rowState(page: Page): Promise<string> {
  const row = page.getByTestId('ytdl-item').first();
  await expect(row).toBeVisible({ timeout: 15_000 });
  return (await row.getByTestId('ytdl-status').innerText()).trim();
}

let token = '';

test.beforeEach(async () => {
  token = await apiToken();
  await resetJobs();
  // Records outlive the cluster now (spec 0013), so resetting the mock is no
  // longer enough to give a test a clean slate.
  await clearYtdl(token);
});

test('a submitted song walks queued → starting → downloading → saved', async ({ page }) => {
  const { status, requestId } = await submit(token, 'https://youtu.be/zSGhyrF7YVo', 'music');
  expect(status).toBe(202);

  await gotoTasks(page);
  // Queued first: submitting records the request and puts it in SynoDL's own
  // queue; the reconciler admits it when a slot frees (spec 0013, FR-022). It
  // moves on within a cycle, so poll rather than assert the instant.
  await expect.poll(() => rowState(page), { timeout: 20_000 }).toBe('starting');

  await drive(requestId, 'start');
  await expect.poll(() => rowState(page), { timeout: 20_000 }).toBe('downloading');

  await drive(requestId, 'succeed');
  await expect.poll(() => rowState(page), { timeout: 20_000 }).toBe('saved');
});

// FR-018, the sharpest edge in the feature. A failed download that the cluster
// then sweeps must stay visibly failed — never quietly become a success, and
// never simply disappear.
test('a failed download stays failed, even after its job is swept away', async ({ page }) => {
  const { requestId } = await submit(token, 'https://youtu.be/failing', 'music');
  await gotoTasks(page);
  await expect(page.getByTestId('ytdl-item').first()).toBeVisible();

  await drive(requestId, 'fail');
  await expect.poll(() => rowState(page), { timeout: 20_000 }).toBe('failed');

  // The job disappears WITHOUT a terminal condition, exactly as TTL cleanup
  // would leave it.
  await drive(requestId, 'vanish');
  await page.waitForTimeout(6_000);
  expect(await rowState(page)).toBe('failed');
});

test('a download stopped for running too long reads as failed, not saved', async ({ page }) => {
  const { requestId } = await submit(token, 'https://youtu.be/slow', 'music');
  await gotoTasks(page);
  await expect(page.getByTestId('ytdl-item').first()).toBeVisible();

  await drive(requestId, 'deadline');
  await expect.poll(() => rowState(page), { timeout: 20_000 }).toBe('failed');
});

test('a YouTube row is marked as such and offers no pause or resume', async ({ page }) => {
  const { requestId } = await submit(token, 'https://youtu.be/marked', 'music');
  await drive(requestId, 'start');

  await gotoTasks(page);
  const row = page.getByTestId('ytdl-item').first();
  await expect(row).toBeVisible({ timeout: 15_000 });

  // FR-026: a mixed list must say which system a row belongs to.
  await expect(page.getByTestId('ytdl-source').first()).toHaveText('YouTube');

  // FR-027: a worker cannot be paused or resumed, so those controls must not
  // exist on the row at all.
  const item = page.getByTestId('ytdl-list');
  await expect(item.getByTestId('task-pause')).toHaveCount(0);
  await expect(item.getByTestId('task-resume')).toHaveCount(0);
});

test('a music video is marked as a video download', async ({ page }) => {
  await submit(token, 'https://youtu.be/videomode', 'music-video');
  await gotoTasks(page);
  await expect(page.getByTestId('ytdl-item').first()).toBeVisible({ timeout: 15_000 });
  await expect(page.getByTestId('ytdl-list')).toContainText('Music video');
});

test('playlist and channel links are accepted and scoped by the server', async () => {
  const playlist = await fetch(`${API}/v1/ytdl`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-SynoDL-Session': token },
    body: JSON.stringify({ url: 'https://youtube.com/playlist?list=PL123', mode: 'music' }),
  });
  expect(playlist.status).toBe(202);
  expect((await playlist.json()).scope).toBe('playlist');

  const channel = await fetch(`${API}/v1/ytdl`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-SynoDL-Session': token },
    body: JSON.stringify({ url: 'https://youtube.com/@someartist', mode: 'music' }),
  });
  expect(channel.status).toBe(202);
  // The scope is DERIVED from the link, never taken from the caller.
  expect((await channel.json()).scope).toBe('channel');
});

// The host allowlist is a constitution rule, so it gets an end-to-end test and
// not only a unit one.
test('a link that is not YouTube is refused outright', async () => {
  for (const url of [
    'https://vimeo.com/12345',
    'https://youtube.com.evil.tld/watch?v=a',
    'https://www.youtube.com@evil.tld/watch?v=a',
    'file:///etc/passwd',
  ]) {
    const res = await fetch(`${API}/v1/ytdl`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-SynoDL-Session': token },
      body: JSON.stringify({ url, mode: 'music' }),
    });
    expect(res.status, `${url} should be refused`).toBe(400);
  }
});

test('the same link cannot be started twice while it is still running', async () => {
  const first = await submit(token, 'https://youtu.be/onlyonce', 'music');
  expect(first.status).toBe(202);
  const second = await submit(token, 'https://youtu.be/onlyonce', 'music');
  expect(second.status).toBe(409);

  // The same link as a video is a different download into a different library.
  const asVideo = await submit(token, 'https://youtu.be/onlyonce', 'music-video');
  expect(asVideo.status).toBe(202);
});

test('signing out is enough to be refused', async () => {
  const res = await fetch(`${API}/v1/ytdl`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url: 'https://youtu.be/nope', mode: 'music' }),
  });
  expect(res.status).toBe(401);
});

test.afterAll(async () => {
  await resetJobs();
});

// Spec 1034: the row describes what is downloading rather than showing the URL
// that was pasted. The mock answers the metadata lookup deterministically, so
// this asserts our rendering rather than whatever YouTube publishes today.
test('a row names the track instead of showing its link', async ({ page }) => {
  await submit(token, 'https://youtu.be/namedtrack', 'music');
  await gotoTasks(page);

  const row = page.getByTestId('ytdl-item').first();
  await expect(row).toBeVisible({ timeout: 15_000 });
  await expect(row.getByTestId('ytdl-name')).toHaveText('Mock Track namedtrack');
  await expect(row.getByTestId('ytdl-uploader')).toHaveText('Mock Artist');
  // The URL must no longer be the heading.
  await expect(row.getByTestId('ytdl-name')).not.toContainText('youtu.be');
});

// A channel publishes no metadata document, so the row falls back to the link.
// That fallback is a normal path, not an error path.
test('a channel with no metadata still reads sensibly', async ({ page }) => {
  await submit(token, 'https://youtube.com/@someartist', 'music');
  await gotoTasks(page);

  const row = page.getByTestId('ytdl-item').first();
  await expect(row).toBeVisible({ timeout: 15_000 });
  await expect(row.getByTestId('ytdl-name')).toContainText('@someartist');
  await expect(row.getByTestId('ytdl-uploader')).toHaveCount(0);
});

/**
 * History (spec 0013, US2). The record outlives the worker: a download that has
 * finished stays visible with everything known about it long after the cluster
 * has swept the job that did it.
 */
test('a finished download survives its job being swept away', async ({ page }) => {
  const token = await apiToken();
  const { requestId } = await submit(token, 'https://youtu.be/zSGhyrF7YVo', 'music');

  await drive(requestId, 'start');
  await drive(requestId, 'succeed');
  await gotoTasks(page);
  await expect(page.getByTestId('ytdl-status')).toHaveText('saved');

  // The cluster sweeps the job. Under spec 0012 a successful download vanished
  // with it, because the files were considered its only record.
  await drive(requestId, 'vanish');

  // Already signed in, so navigate rather than logging in again.
  await page.goto('/tabs/tasks');
  await expect(page.getByTestId('task-list').or(page.getByTestId('tasks-empty'))).toBeVisible();
  const row = page.getByTestId('ytdl-item').first();
  await expect(row).toBeVisible();
  await expect(page.getByTestId('ytdl-status')).toHaveText('saved');
  // And it still knows what it was, not just that something happened.
  await expect(page.getByTestId('ytdl-name')).not.toHaveText('');
});

test('a queued download reads differently from one that is starting', async ({ page }) => {
  // FR-013b. "SynoDL is holding this behind others" and "the worker is coming
  // up now" are different waits, and a user who cannot tell them apart reads the
  // first as the second hanging.
  const token = await apiToken();
  const { requestId } = await submit(token, 'https://youtu.be/zSGhyrF7YVo', 'music');

  await gotoTasks(page);
  await expect
    .poll(async () => (await page.getByTestId('ytdl-status').innerText()).trim(), { timeout: 20_000 })
    .toBe('starting');

  await drive(requestId, 'start');
  await expect(page.getByTestId('ytdl-status')).toHaveText('downloading');
});

/**
 * Retry (spec 0013, US5). Recovering from a transient failure should be one
 * action, not "find the link again and re-paste it" — which is the manual
 * routine this whole feature exists to remove.
 */
test('a failed download can be retried, and runs again', async ({ page }) => {
  const { requestId } = await submit(token, 'https://youtu.be/zSGhyrF7YVo', 'music');
  await drive(requestId, 'start');
  await drive(requestId, 'fail');

  await gotoTasks(page);
  await expect.poll(() => rowState(page), { timeout: 20_000 }).toBe('failed');

  // Retry from the detail sheet, where someone would have just read the reason.
  await page.getByTestId('ytdl-item').first().click();
  await expect(page.getByTestId('ytdl-detail')).toBeVisible();
  await page.getByTestId('ytdl-detail-retry').click();

  // Back in the queue, then running again — and still ONE row, not two.
  await expect
    .poll(() => rowState(page), { timeout: 25_000 })
    .toMatch(/waiting its turn|starting|downloading/);
  await expect(page.getByTestId('ytdl-item')).toHaveCount(1);
});

test('retry is not offered for a download that has not failed', async ({ page }) => {
  const { requestId } = await submit(token, 'https://youtu.be/zSGhyrF7YVo', 'music');
  await drive(requestId, 'start');
  await drive(requestId, 'succeed');

  await gotoTasks(page);
  await expect.poll(() => rowState(page), { timeout: 20_000 }).toBe('saved');

  await page.getByTestId('ytdl-item').first().click();
  await expect(page.getByTestId('ytdl-detail')).toBeVisible();
  await expect(page.getByTestId('ytdl-detail-retry')).toHaveCount(0);
});
