/**
 * A playlist or channel becomes its own items (spec 0013, US6).
 *
 * Spec 0012 fetched a channel as ONE bulk job. That could not say which item was
 * downloading, could not report which item failed, and could not be retried at
 * item granularity — so a channel was all-or-nothing in a way nothing else in
 * the app is.
 *
 * Expansion is driven here through the mock cluster's emit control, so the path
 * under test is the real one: enumeration worker → its pod log → the entries
 * SynoDL parses → one queued download per entry.
 */
import { expect, test, type Page } from '@playwright/test';
import { apiToken, clearYtdl, login } from './helpers';

const SF_PORT = Number(process.env.SYNODL_E2E_SF_PORT) || 8283;
const K8S = `http://localhost:${process.env.SYNODL_E2E_SF_K8S_PORT || 8296}`;
const API = `http://localhost:${SF_PORT}`;
const ENTRY = '[synodl-entry]';

type Row = { requestId: string; kind: string; state: string; title?: string; counts?: { total: number } };

async function submit(token: string, url: string): Promise<{ requestId: string; kind: string }> {
  const res = await fetch(`${API}/v1/ytdl`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-SynoDL-Session': token },
    body: JSON.stringify({ url, mode: 'music' }),
  });
  expect(res.status).toBe(202);
  return (await res.json()) as { requestId: string; kind: string };
}

async function rows(token: string): Promise<Row[]> {
  const res = await fetch(`${API}/v1/ytdl?limit=200`, { headers: { 'X-SynoDL-Session': token } });
  return ((await res.json()) as { downloads: Row[] }).downloads;
}

async function items(token: string, groupId: string): Promise<Row[]> {
  const res = await fetch(`${API}/v1/ytdl/${groupId}/items`, {
    headers: { 'X-SynoDL-Session': token },
  });
  if (!res.ok) return [];
  return ((await res.json()) as { items: Row[] }).items;
}

/** Feed the enumeration worker's output, once the cluster has started it. */
async function emitEntries(groupId: string, ids: string[]): Promise<void> {
  const name = `synodl-ytdl-${groupId}-expand`;
  const deadline = Date.now() + 25_000;
  for (;;) {
    const lines = ids
      .map((id) => `${ENTRY} id=${id} uploader=Lo-fi Beats title=Track ${id}`)
      .join('\n');
    const res = await fetch(`${K8S}/__mock/jobs/${name}/emit`, { method: 'POST', body: lines });
    if (res.ok) {
      // Then let it finish, so the reconciler reads what it printed.
      const done = await fetch(`${K8S}/__mock/jobs/${name}/succeed`, { method: 'POST' });
      if (done.ok) return;
    }
    if (Date.now() > deadline) throw new Error('enumeration worker never appeared');
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

test('a channel expands into one download per item', async () => {
  const { requestId, kind } = await submit(token, 'https://www.youtube.com/@lofi');
  expect(kind, 'a channel is a group, not a single download').toBe('group');

  await emitEntries(requestId, ['aaaaaaaaaaa', 'bbbbbbbbbbb', 'ccccccccccc']);

  await expect.poll(() => items(token, requestId).then((i) => i.length), { timeout: 30_000 }).toBe(3);

  // Each item is a download in its own right.
  for (const it of await items(token, requestId)) {
    expect(it.kind).toBe('item');
    expect(['queued', 'scheduled', 'downloading', 'completed', 'failed']).toContain(it.state);
  }
});

test('the Tasks list gains ONE row for a channel, not one per item', async ({ page }) => {
  // SC-004a. With no ceiling on expansion, a flat list would push every other
  // download off the screen — which is what the group row prevents.
  const { requestId } = await submit(token, 'https://www.youtube.com/@lofi');
  await emitEntries(requestId, ['aaaaaaaaaaa', 'bbbbbbbbbbb', 'ccccccccccc', 'ddddddddddd']);
  await expect.poll(() => items(token, requestId).then((i) => i.length), { timeout: 30_000 }).toBe(4);

  const top = await rows(token);
  expect(top).toHaveLength(1);
  expect(top[0].requestId).toBe(requestId);
  expect(top[0].kind).toBe('group');

  await gotoTasks(page);
  await expect(page.getByTestId('ytdl-item')).toHaveCount(1);
  // And the row says how its contents are getting on.
  await expect(page.getByTestId('ytdl-group-summary')).toBeVisible();
});

test('opening the group row shows its items', async ({ page }) => {
  const { requestId } = await submit(token, 'https://www.youtube.com/@lofi');
  await emitEntries(requestId, ['aaaaaaaaaaa', 'bbbbbbbbbbb']);
  await expect.poll(() => items(token, requestId).then((i) => i.length), { timeout: 30_000 }).toBe(2);

  await gotoTasks(page);
  await page.getByTestId('ytdl-item').first().click();
  await expect(page.getByTestId('ytdl-group-items')).toBeVisible();
  await expect(page.getByTestId('ytdl-group-item')).toHaveCount(2);
});

test('re-submitting a channel queues only what is new', async () => {
  // FR-020. Checked at expansion, before anything is queued, so a re-run does
  // not create rows that would immediately finish having done nothing.
  const first = await submit(token, 'https://www.youtube.com/@lofi');
  await emitEntries(first.requestId, ['aaaaaaaaaaa', 'bbbbbbbbbbb']);
  await expect
    .poll(() => items(token, first.requestId).then((i) => i.length), { timeout: 30_000 })
    .toBe(2);

  // Finish them both, so they count as held.
  for (const it of await items(token, first.requestId)) {
    const deadline = Date.now() + 25_000;
    for (;;) {
      const res = await fetch(`${K8S}/__mock/jobs/${it.requestId}/succeed`, { method: 'POST' });
      if (res.ok || Date.now() > deadline) break;
      await new Promise((r) => setTimeout(r, 500));
    }
  }
  await expect
    .poll(
      async () => (await items(token, first.requestId)).filter((i) => i.state === 'completed').length,
      { timeout: 30_000 },
    )
    .toBe(2);

  // Wait for the GROUP itself to finish, not just its items. Re-submitting a
  // channel that is still running is a duplicate and is refused — correctly —
  // so the second submission has to come after the first has settled.
  await expect
    .poll(
      async () => (await rows(token)).find((r) => r.requestId === first.requestId)?.state,
      { timeout: 30_000 },
    )
    .toBe('completed');

  // Same channel again, now listing one extra item.
  const second = await submit(token, 'https://www.youtube.com/@lofi');
  await emitEntries(second.requestId, ['aaaaaaaaaaa', 'bbbbbbbbbbb', 'zzzzzzzzzzz']);

  await expect
    .poll(() => items(token, second.requestId).then((i) => i.length), { timeout: 30_000 })
    .toBe(1);
  const fresh = await items(token, second.requestId);
  expect(fresh[0].title).toContain('zzzzzzzzzzz');
});

test('an open group sheet does not refetch its items on every poll', async ({ page }) => {
  // The sheet used to clear and refetch its items every few seconds: its
  // watcher's getter returned a fresh array each run, so Vue's reference
  // comparison fired on every re-render of the polled list.
  //
  // Counting REQUESTS is the instrument, not sampling the rendered rows —
  // the clear-and-refetch window is milliseconds wide, so watching for an
  // empty list misses it and the test passes with the bug present.
  const { requestId } = await submit(token, 'https://www.youtube.com/playlist?list=PLtest');
  await emitEntries(requestId, ['aaaaaaaaaaa', 'bbbbbbbbbbb', 'ccccccccccc']);
  await expect.poll(() => items(token, requestId).then((i) => i.length), { timeout: 30_000 }).toBe(3);

  await gotoTasks(page);

  let itemFetches = 0;
  page.on('request', (r) => {
    if (/\/v1\/ytdl\/[^/]+\/items/.test(r.url())) itemFetches += 1;
  });

  await page.getByTestId('ytdl-item').first().click();
  await expect(page.getByTestId('ytdl-group-items')).toBeVisible();
  await expect(page.getByTestId('ytdl-group-item')).toHaveCount(3);

  const afterOpen = itemFetches;
  expect(afterOpen, 'opening the sheet should fetch the items once').toBeGreaterThan(0);

  // Nothing about the group changes for the next 15s — no item finishes, no
  // count moves — so there is nothing to re-read.
  await page.waitForTimeout(15_000);

  const extra = itemFetches - afterOpen;
  expect(
    extra,
    `items were refetched ${extra} more times while nothing changed; the sheet is reloading on every poll`,
  ).toBeLessThanOrEqual(1);

  await expect(page.getByTestId('ytdl-group-item')).toHaveCount(3);
});
