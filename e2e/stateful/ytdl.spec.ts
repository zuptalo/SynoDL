/**
 * YouTube downloads (spec 0012).
 *
 * These run against the fake Jobs API on :8296, which downloads nothing and
 * moves a job only when told to. That determinism is the point: the states this
 * feature reports are the whole feature, so each one is driven explicitly
 * rather than waited for.
 */
import { expect, test, type Page } from '@playwright/test';
import { apiToken, login } from './helpers';

const SF_PORT = Number(process.env.SYNODL_E2E_SF_PORT) || 8283;
const K8S = `http://localhost:${process.env.SYNODL_E2E_SF_K8S_PORT || 8296}`;
const API = `http://localhost:${SF_PORT}`;

async function resetJobs(): Promise<void> {
  await fetch(`${K8S}/__mock/reset`, { method: 'POST' });
}

/** Drive one job to a lifecycle state, addressed by its request id. */
async function drive(requestId: string, action: string): Promise<void> {
  const res = await fetch(`${K8S}/__mock/jobs/${requestId}/${action}`, { method: 'POST' });
  if (!res.ok) throw new Error(`drive ${action} failed: ${res.status}`);
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
});

test('a submitted song walks queued → downloading → saved', async ({ page }) => {
  const { status, requestId } = await submit(token, 'https://youtu.be/zSGhyrF7YVo', 'music');
  expect(status).toBe(202);

  await gotoTasks(page);
  expect(await rowState(page)).toBe('queued');

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
