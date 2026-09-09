/**
 * YouTube downloads update as they happen (spec 1038).
 *
 * Reported from use: "the status of the downloads seem to be more like a pull
 * flow than a sse flow, I see the sub tasks under a playlist getting refreshed
 * every 5 seconds or so."
 *
 * These tests count REQUESTS as well as watching the screen, and that is
 * deliberate: a row that updates says nothing about whether it updated because
 * the server pushed it or because the client asked again. Only the request count
 * tells those apart, and it is the difference the whole spec is about.
 */
import { expect, test, type Page } from '@playwright/test';
import { apiToken, clearYtdl, login } from './helpers';

const SF_PORT = Number(process.env.SYNODL_E2E_SF_PORT) || 8283;
const K8S = `http://localhost:${process.env.SYNODL_E2E_SF_K8S_PORT || 8296}`;
const API = `http://localhost:${SF_PORT}`;
const SENTINEL = '[synodl]';
const ENTRY = '[synodl-entry]';

async function submit(token: string, url: string): Promise<string> {
  const res = await fetch(`${API}/v1/ytdl`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-SynoDL-Session': token },
    body: JSON.stringify({ url, mode: 'music' }),
  });
  expect(res.status).toBe(202);
  return ((await res.json()) as { requestId: string }).requestId;
}

async function drive(requestId: string, action: string): Promise<void> {
  const deadline = Date.now() + 25_000;
  for (;;) {
    const res = await fetch(`${K8S}/__mock/jobs/${requestId}/${action}`, { method: 'POST' });
    if (res.ok) return;
    if (res.status !== 404 || Date.now() > deadline) {
      throw new Error(`drive ${action}: ${res.status}`);
    }
    await new Promise((r) => setTimeout(r, 500));
  }
}

async function emit(requestId: string, line: string): Promise<void> {
  const deadline = Date.now() + 25_000;
  for (;;) {
    const res = await fetch(`${K8S}/__mock/jobs/${requestId}/emit`, { method: 'POST', body: line });
    if (res.ok) return;
    if (res.status !== 404 || Date.now() > deadline) throw new Error(`emit: ${res.status}`);
    await new Promise((r) => setTimeout(r, 500));
  }
}

async function emitEntries(groupId: string, ids: string[]): Promise<void> {
  const name = `synodl-ytdl-${groupId}-expand`;
  const deadline = Date.now() + 25_000;
  for (;;) {
    const lines = ids.map((id) => `${ENTRY} id=${id} uploader=Anyma title=Track ${id}`).join('\n');
    const res = await fetch(`${K8S}/__mock/jobs/${name}/emit`, { method: 'POST', body: lines });
    if (res.ok) {
      const done = await fetch(`${K8S}/__mock/jobs/${name}/succeed`, { method: 'POST' });
      if (done.ok) return;
    }
    if (Date.now() > deadline) throw new Error('enumeration worker never appeared');
    await new Promise((r) => setTimeout(r, 500));
  }
}

async function groupItems(groupId: string): Promise<{ requestId: string }[]> {
  const res = await fetch(`${API}/v1/ytdl/${groupId}/items`, {
    headers: { 'X-SynoDL-Session': token },
  });
  if (!res.ok) return [];
  return ((await res.json()) as { items: { requestId: string }[] }).items;
}

async function gotoTasks(page: Page): Promise<void> {
  await login(page);
  await page.goto('/tabs/tasks');
  await expect(page.getByTestId('task-list').or(page.getByTestId('tasks-empty'))).toBeVisible();
}

/** Count list fetches — the asking this feature exists to remove. */
function countListFetches(page: Page): () => number {
  let n = 0;
  page.on('request', (r) => {
    const u = new URL(r.url());
    if (u.pathname === '/v1/ytdl') n += 1;
  });
  return () => n;
}

let token = '';

test.beforeEach(async () => {
  token = await apiToken();
  await fetch(`${K8S}/__mock/reset`, { method: 'POST' });
  await clearYtdl(token);
});

test('a download changes state on screen without the list being asked again', async ({ page }) => {
  const requestId = await submit(token, 'https://youtu.be/zSGhyrF7YVo');
  await drive(requestId, 'start');

  await gotoTasks(page);
  await expect(page.getByTestId('ytdl-status')).toHaveText('downloading', { timeout: 20_000 });

  // Count from HERE: the initial paint legitimately fetches, and so does the
  // fetch on `ready` that closes the connect gap.
  const listFetches = countListFetches(page);

  await emit(requestId, `${SENTINEL} status=downloading downloaded=60 total=100`);
  await expect(page.getByTestId('ytdl-percent')).toHaveText('60%', { timeout: 20_000 });

  await drive(requestId, 'succeed');
  await expect(page.getByTestId('ytdl-status')).toHaveText('saved', { timeout: 20_000 });

  expect(
    listFetches(),
    'the row followed the download by re-reading the whole list, which is the polling this replaced',
  ).toBe(0);
});

test('an idle list asks for nothing at all', async ({ page }) => {
  // SC-001. The complaint was visible refreshing with nothing happening.
  await submit(token, 'https://youtu.be/zSGhyrF7YVo');
  await gotoTasks(page);
  await expect(page.getByTestId('ytdl-item')).toHaveCount(1, { timeout: 20_000 });

  const listFetches = countListFetches(page);
  await page.waitForTimeout(20_000); // several old poll intervals, and a heartbeat

  expect(listFetches(), 'the list was re-read while nothing was happening').toBe(0);
});

test('a track inside an open playlist updates without the sheet re-reading it', async ({ page }) => {
  // US2, and the case that was actually noticed. The sheet used to re-fetch its
  // whole first page every few seconds.
  const groupId = await submit(token, 'https://www.youtube.com/playlist?list=PLtest');
  await emitEntries(groupId, ['aaaaaaaaaaa', 'bbbbbbbbbbb', 'ccccccccccc']);
  // Expansion is a worker of its own, so wait for it to have produced the items
  // before opening the sheet — otherwise the test is measuring the expansion,
  // not the updating.
  await expect
    .poll(() => groupItems(groupId).then((i) => i.length), { timeout: 30_000 })
    .toBe(3);

  await gotoTasks(page);
  await page.getByTestId('ytdl-item').first().click();
  await expect(page.getByTestId('ytdl-group-items')).toBeVisible();
  await expect(page.getByTestId('ytdl-group-item')).toHaveCount(3, { timeout: 20_000 });

  let itemFetches = 0;
  page.on('request', (r) => {
    if (/\/v1\/ytdl\/[^/]+\/items/.test(r.url())) itemFetches += 1;
  });

  // Find whichever item the queue admitted and drive it to a reading.
  const first = (await groupItems(groupId))[0].requestId;
  await drive(first, 'start');
  await emit(first, `${SENTINEL} status=downloading downloaded=35 total=100`);

  await expect(page.getByTestId('ytdl-group-item').getByTestId('ytdl-percent').first()).toHaveText(
    '35%',
    { timeout: 20_000 },
  );

  expect(
    itemFetches,
    `the sheet refetched its items ${itemFetches} times; it should have been told what changed`,
  ).toBe(0);
});

test('with no stream, the list still updates by asking', async ({ page }) => {
  // FR-008 / SC-005. A blocked stream must degrade to the old behaviour, not to
  // a frozen or blank list.
  await page.route('**/v1/ytdl/stream', (route) => route.abort());

  const requestId = await submit(token, 'https://youtu.be/zSGhyrF7YVo');
  await drive(requestId, 'start');

  await gotoTasks(page);
  await expect(page.getByTestId('ytdl-status')).toHaveText('downloading', { timeout: 20_000 });

  await drive(requestId, 'succeed');
  await expect(page.getByTestId('ytdl-status')).toHaveText('saved', { timeout: 30_000 });
});
