/**
 * A YouTube download reports real progress (spec 0013, US3).
 *
 * Spec 0012 shipped without this and said why: the worker had no channel back
 * to SynoDL. It has one — its own output — and these tests drive that output
 * directly through the mock cluster's emit control, so what is under test is the
 * whole path: worker line → pod log → reconciler → held reading → the bar.
 *
 * The lines emitted here are the format SynoDL asked the worker to print. That
 * is the point of the sentinel: a line the app acts on is a line it specified.
 */
import { expect, test, type Page } from '@playwright/test';
import { apiToken, clearYtdl, login } from './helpers';

const SF_PORT = Number(process.env.SYNODL_E2E_SF_PORT) || 8283;
const K8S = `http://localhost:${process.env.SYNODL_E2E_SF_K8S_PORT || 8296}`;
const API = `http://localhost:${SF_PORT}`;
const SENTINEL = '[synodl]';

async function submit(token: string, url: string): Promise<string> {
  const res = await fetch(`${API}/v1/ytdl`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-SynoDL-Session': token },
    body: JSON.stringify({ url, mode: 'music' }),
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

/**
 * Append a line to the worker's output, as the worker itself would.
 *
 * Retries on 404 for the same reason drive() does: the worker exists only once
 * the download has been admitted from the queue.
 */
async function emit(requestId: string, line: string): Promise<void> {
  const deadline = Date.now() + 25_000;
  for (;;) {
    const res = await fetch(`${K8S}/__mock/jobs/${requestId}/emit`, { method: 'POST', body: line });
    if (res.ok) return;
    if (res.status !== 404 || Date.now() > deadline) throw new Error(`emit: ${res.status}`);
    await new Promise((r) => setTimeout(r, 500));
  }
}

async function gotoTasks(page: Page): Promise<void> {
  await login(page);
  await page.goto('/tabs/tasks');
  await expect(page.getByTestId('task-list').or(page.getByTestId('tasks-empty'))).toBeVisible();
}

/** The percentage shown on the row, or null when none is shown. */
async function shownPercent(page: Page): Promise<number | null> {
  const label = page.getByTestId('ytdl-percent');
  if ((await label.count()) === 0) return null;
  const text = (await label.first().innerText()).trim();
  const m = /(\d+)%/.exec(text);
  return m ? Number(m[1]) : null;
}

let token = '';

test.beforeEach(async () => {
  token = await apiToken();
  await fetch(`${K8S}/__mock/reset`, { method: 'POST' });
  await clearYtdl(token);
});

test('a running download shows a bar that advances', async ({ page }) => {
  const requestId = await submit(token, 'https://youtu.be/zSGhyrF7YVo');
  await drive(requestId, 'start');
  await emit(requestId, `${SENTINEL} status=downloading downloaded=20 total=100`);

  await gotoTasks(page);
  await expect(page.getByTestId('ytdl-progress')).toBeVisible({ timeout: 20_000 });
  await expect.poll(() => shownPercent(page), { timeout: 20_000 }).toBe(20);

  await emit(requestId, `${SENTINEL} status=downloading downloaded=70 total=100`);
  await expect.poll(() => shownPercent(page), { timeout: 20_000 }).toBe(70);
});

test('the bar never goes backwards when a reading arrives out of order', async ({ page }) => {
  // FR-012. The reconciler re-reads a window of the log each cycle, so the last
  // line of one stream can be seen after the first line of the next. A bar that
  // visibly restarts would read as the download starting over.
  const requestId = await submit(token, 'https://youtu.be/zSGhyrF7YVo');
  await drive(requestId, 'start');
  await emit(requestId, `${SENTINEL} status=downloading downloaded=80 total=100`);

  await gotoTasks(page);
  await expect.poll(() => shownPercent(page), { timeout: 20_000 }).toBe(80);

  // An older, lower reading turns up.
  await emit(requestId, `${SENTINEL} status=downloading downloaded=10 total=100`);
  await page.waitForTimeout(6_000);
  const after = await shownPercent(page);
  expect(after, 'progress must never move backwards').toBeGreaterThanOrEqual(80);
});

test('a download whose output cannot be read shows no bar, and is not failed', async ({ page }) => {
  // FR-013. Losing the reading is not the download failing, and a bar pinned at
  // zero would say it had stalled. Showing nothing is the honest answer.
  const requestId = await submit(token, 'https://youtu.be/zSGhyrF7YVo');
  await drive(requestId, 'start'); // running, but the worker has printed nothing

  await gotoTasks(page);
  await expect(page.getByTestId('ytdl-status')).toHaveText('downloading', { timeout: 20_000 });
  expect(await shownPercent(page)).toBeNull();
  await expect(page.getByTestId('ytdl-progress')).toHaveCount(0);
});

test('a finished download reports whether lyrics were saved, and in what language', async ({
  page,
}) => {
  // FR-011. This fact exists ONLY in the worker's output — the server never
  // mounts the media library — so it has to be captured before the cluster
  // sweeps the pod that said it.
  const requestId = await submit(token, 'https://youtu.be/zSGhyrF7YVo');
  await drive(requestId, 'start');
  await emit(
    requestId,
    '[info] Writing video subtitles to: /out/Queen/Singles/Bohemian Rhapsody.en-orig.lrc',
  );

  await gotoTasks(page);
  await expect(page.getByTestId('ytdl-status')).toHaveText('downloading', { timeout: 20_000 });

  // Give the reconciler a cycle to read it, then check the record kept it.
  await expect
    .poll(
      async () => {
        const res = await fetch(`${API}/v1/ytdl`, { headers: { 'X-SynoDL-Session': token } });
        const { downloads } = (await res.json()) as {
          downloads: { requestId: string; hasLyrics?: boolean; lyricsLang?: string }[];
        };
        const row = downloads.find((d) => d.requestId === requestId);
        return row ? `${row.hasLyrics ?? false}/${row.lyricsLang ?? ''}` : 'missing';
      },
      { timeout: 20_000 },
    )
    .toBe('true/en');
});
