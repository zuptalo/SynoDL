/**
 * Four at a time, the rest wait their turn (spec 0013, US7).
 *
 * Spec 0012 had no queue: concurrency was bounded by the cluster, which is a
 * real bound but an invisible one — a user could not see it, could not reorder
 * it, and could not tell it from a download that was simply slow. Once a channel
 * can expand with no ceiling, it also means dropping hundreds of jobs on the
 * orchestrator at once.
 *
 * These assert through the API rather than the UI: what is under test is which
 * downloads the server chose to start, and counting rows in a browser would only
 * make that harder to read.
 */
import { expect, test } from '@playwright/test';
import { apiToken, clearYtdl } from './helpers';

const SF_PORT = Number(process.env.SYNODL_E2E_SF_PORT) || 8283;
const K8S = `http://localhost:${process.env.SYNODL_E2E_SF_K8S_PORT || 8296}`;
const API = `http://localhost:${SF_PORT}`;

type Row = { requestId: string; state: string };

async function submit(token: string, url: string): Promise<string> {
  const res = await fetch(`${API}/v1/ytdl`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-SynoDL-Session': token },
    body: JSON.stringify({ url, mode: 'music' }),
  });
  expect(res.status, `submit ${url}`).toBe(202);
  return ((await res.json()) as { requestId: string }).requestId;
}

async function rows(token: string): Promise<Row[]> {
  const res = await fetch(`${API}/v1/ytdl?limit=200`, { headers: { 'X-SynoDL-Session': token } });
  expect(res.ok).toBe(true);
  return ((await res.json()) as { downloads: Row[] }).downloads;
}

async function countIn(token: string, states: string[]): Promise<number> {
  return (await rows(token)).filter((r) => states.includes(r.state)).length;
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

let token = '';

test.beforeEach(async () => {
  token = await apiToken();
  await fetch(`${K8S}/__mock/reset`, { method: 'POST' });
  await clearYtdl(token);
});

test('only four run at once; the rest wait their turn', async () => {
  const ids: string[] = [];
  for (let i = 0; i < 9; i++) ids.push(await submit(token, `https://youtu.be/queued${i}`));

  // The reconciler admits on its own clock, so wait for it to settle rather
  // than assuming an instant.
  await expect
    .poll(() => countIn(token, ['scheduled', 'downloading']), { timeout: 25_000 })
    .toBe(4);

  // And it stays at four — nothing has finished, so nothing else may start.
  await new Promise((r) => setTimeout(r, 6_000));
  expect(await countIn(token, ['scheduled', 'downloading'])).toBe(4);
  expect(await countIn(token, ['queued'])).toBe(5);
});

test('a finish frees a slot, whether it succeeded or failed', async () => {
  const ids: string[] = [];
  for (let i = 0; i < 6; i++) ids.push(await submit(token, `https://youtu.be/slot${i}`));

  await expect
    .poll(() => countIn(token, ['scheduled', 'downloading']), { timeout: 25_000 })
    .toBe(4);

  // Whichever four were admitted, finish two of them — one each way, because
  // the limit is about what is RUNNING, not about what worked.
  const running = (await rows(token)).filter((r) => r.state === 'scheduled' || r.state === 'downloading');
  await drive(running[0].requestId, 'succeed');
  await drive(running[1].requestId, 'fail');

  await expect
    .poll(() => countIn(token, ['queued']), { timeout: 25_000 })
    .toBe(0);
  await expect
    .poll(() => countIn(token, ['scheduled', 'downloading']), { timeout: 25_000 })
    .toBe(4);
});

test('a queued download can be called off before it ever starts', async () => {
  // FR-005b. Dismissing a waiting download removes it from the queue, so it
  // never starts — the point of being able to call off a mistake.
  const ids: string[] = [];
  for (let i = 0; i < 9; i++) ids.push(await submit(token, `https://youtu.be/callable${i}`));

  await expect
    .poll(() => countIn(token, ['queued']), { timeout: 25_000 })
    .toBeGreaterThan(0);

  const waiting = (await rows(token)).filter((r) => r.state === 'queued');
  const victim = waiting[waiting.length - 1].requestId;
  const res = await fetch(`${API}/v1/ytdl/${victim}`, {
    method: 'DELETE',
    headers: { 'X-SynoDL-Session': token },
  });
  expect(res.status).toBe(204);

  await new Promise((r) => setTimeout(r, 6_000));
  expect((await rows(token)).some((r) => r.requestId === victim)).toBe(false);
});
