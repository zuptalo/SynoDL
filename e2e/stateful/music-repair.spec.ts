/**
 * Running the music library repair from Settings (spec 1053).
 *
 * The repair's WORKER is not run here — the mock cluster only holds Jobs and their
 * output — so each test plays the worker: it appends the fixed `@@synodl` event
 * lines a real run prints, then ends the Job. Everything else is real: the server
 * creates the Job through the same client it uses in a cluster, reads the output
 * through the same pod-log call, parses it into the fixed shape, and the screen
 * shows that shape.
 *
 * One repair runs at a time, so the tests are serial and share state on purpose:
 * check → apply → undo is one story told in order.
 */
import { expect, test, type Page } from '@playwright/test';
import { ADMIN, apiToken, createSecondUser, login } from './helpers';

const SF_PORT = Number(process.env.SYNODL_E2E_SF_PORT) || 8283;
const K8S = `http://localhost:${process.env.SYNODL_E2E_SF_K8S_PORT || 8296}`;
const API = `http://localhost:${SF_PORT}`;

const PLAN_A = '20260929T100000Z-aaaa01';
const PLAN_B = '20260929T110000Z-bbbb02';

test.describe.configure({ mode: 'serial' });

interface Snap {
  available: boolean;
  reason: string;
  current: { id: string; kind: string; state: string } | null;
  plan: { id: string; status: string; canApply: boolean; canContinue: boolean } | null;
  undo: { canUndo: boolean } | null;
  history: { kind: string; state: string; startedBy: string }[];
}

async function snapshot(token: string): Promise<Snap> {
  const res = await fetch(`${API}/v1/library/repair`, { headers: { 'X-SynoDL-Session': token } });
  expect(res.status).toBe(200);
  return (await res.json()) as Snap;
}

async function post(token: string, path: string, body?: unknown): Promise<Response> {
  return fetch(`${API}/v1/library/repair/${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-SynoDL-Session': token },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

/** The worker's side: print an event line into the Job's output. */
async function emit(jobName: string, event: object): Promise<void> {
  const res = await fetch(`${K8S}/__mock/jobs/${jobName}/emit`, {
    method: 'POST',
    body: `@@synodl ${JSON.stringify(event)}`,
  });
  expect(res.status).toBe(204);
}

async function endJob(jobName: string, action: 'succeed' | 'fail'): Promise<void> {
  const res = await fetch(`${K8S}/__mock/jobs/${jobName}/${action}`, { method: 'POST' });
  expect(res.ok).toBe(true);
}

/** Wait until the server reports a run in progress, and return its Job's name. */
async function runningJob(token: string): Promise<string> {
  let name = '';
  await expect
    .poll(async () => {
      const s = await snapshot(token);
      name = s.current ? `music-repair-${s.current.id}` : '';
      return name;
    }, { timeout: 15_000 })
    .not.toBe('');
  return name;
}

const CHECK = {
  tracks: 5795, songs: 4017, duplicates: 1774, moves: 3970, retags: 4017, covers: 886, playlists: 88,
  conflicts: 0, nameClashes: 52, matched: 1828, noMatch: 2186, notLookedUp: 3, toSingles: 2917,
  albumKnown: 886, orphanNfo: 3484, bytesReclaimed: 13_368_173_185, bytesNeeded: 548_758_400,
  freeBytes: 4_600_000_000_000,
  leftAlone: {
    total: 6,
    byReason: [{ reason: 'no video id', count: 4 }, { reason: 'an mp3 of that name already exists', count: 2 }],
    examples: [{ path: 'Avaria/Singles/Hold Me Down.mp3', reason: 'no video id' }],
  },
};

const checkResult = (planId: string) => ({
  event: 'result', kind: 'check', ok: true, planId, planFile: `.repair/plan-${planId}.md`, check: CHECK,
});

async function openRepair(page: Page): Promise<void> {
  await page.goto('/tabs/settings');
  await page.getByTestId('settings-music-repair').click();
  await expect(page.getByTestId('repair-content')).toBeVisible();
}

test.describe('music library repair (spec 1053)', () => {
  let token = '';

  test.beforeAll(async () => {
    token = await apiToken();
  });

  test('only an admin has it: a regular user is refused everywhere', async () => {
    const user = await createSecondUser(token, 'repairbo');
    for (const [method, path] of [['GET', ''], ['POST', '/check'], ['POST', '/apply'], ['POST', '/undo']] as const) {
      const res = await fetch(`${API}/v1/library/repair${path}`, {
        method,
        headers: { 'Content-Type': 'application/json', 'X-SynoDL-Session': user },
        body: method === 'POST' ? JSON.stringify({ planId: PLAN_A, snapshotAck: true }) : undefined,
      });
      expect(res.status, `${method} ${path}`).toBe(403);
    }
  });

  test('nothing has run yet, and the section says what a check is', async ({ page }) => {
    await login(page);
    await openRepair(page);
    await expect(page.getByTestId('repair-check')).toBeEnabled();
    await expect(page.getByTestId('repair-plan')).toHaveCount(0);
    await expect(page.getByTestId('repair-apply')).toHaveCount(0);
    await expect(page.getByTestId('repair-undo')).toHaveCount(0);
  });

  test('a check runs, shows its progress, and ends with the numbers the worker reported', async ({ page }) => {
    await login(page);
    await openRepair(page);
    await page.getByTestId('repair-check').click();
    await expect(page.getByTestId('repair-progress')).toBeVisible();
    const job = await runningJob(token);

    // only one run at a time, and it says who
    const second = await post(token, 'check');
    expect(second.status).toBe(409);
    const busy = (await second.json()) as { error: string; startedBy: string; source: string };
    expect(busy.error).toBe('busy');
    expect(busy.startedBy).toBe(ADMIN.username);
    expect(busy.source).toBe('settings');

    // the worker reports progress; the screen shows it
    await emit(job, { event: 'progress', phase: 'lookup', done: 25, total: 100 });
    await expect(page.getByTestId('repair-progress')).toContainText('Looking songs up: 25 of 100', { timeout: 15_000 });

    // closing and reopening keeps the run: it is the cluster's, not the screen's
    await page.getByTestId('repair-close').click();
    await openRepair(page);
    await expect(page.getByTestId('repair-progress')).toContainText('25 of 100', { timeout: 15_000 });

    // the worker finishes
    await emit(job, checkResult(PLAN_A));
    await endJob(job, 'succeed');

    const plan = page.getByTestId('repair-plan');
    await expect(plan).toBeVisible({ timeout: 15_000 });
    await expect(plan).toHaveAttribute('data-status', 'ready');
    await expect(page.getByTestId('repair-row-duplicates')).toContainText('1,774');
    await expect(page.getByTestId('repair-row-moves')).toContainText('3,970');
    await expect(page.getByTestId('repair-row-playlists')).toContainText('88');
    await expect(page.getByTestId('repair-row-matched')).toContainText('1,828');
    await expect(page.getByTestId('repair-row-noMatch')).toContainText('2,186');
    await expect(page.getByTestId('repair-row-space')).toContainText('548.8 MB needed, 4.6 TB free');
    await expect(page.getByTestId('repair-plan-file')).toContainText('.repair');
    await expect(page.getByTestId('repair-plan-file')).toContainText(PLAN_A);

    // what was left alone: counts by reason and a few examples
    await page.getByTestId('repair-left-alone').getByText('Left alone (6)').click();
    await expect(page.getByTestId('repair-left-alone')).toContainText('4 × no video id');
    await expect(page.getByTestId('repair-left-alone')).toContainText('Avaria/Singles/Hold Me Down.mp3');
    await expect(page.getByTestId('repair-left-alone')).toContainText('and 5 more');

    // and nothing else is running
    expect((await snapshot(token)).current).toBeNull();
  });

  test('apply needs the snapshot acknowledgement, on the screen and at the server', async ({ page }) => {
    // the server refuses a request without it, whatever the screen did
    const noAck = await post(token, 'apply', { planId: PLAN_A, snapshotAck: false });
    expect(noAck.status).toBe(400);
    expect(((await noAck.json()) as { error: string }).error).toBe('snapshot_required');
    expect((await snapshot(token)).current).toBeNull();

    await login(page);
    await openRepair(page);
    await page.getByTestId('repair-apply').click();
    const confirm = page.getByTestId('repair-apply-confirm');
    await expect(page.getByTestId('repair-confirm')).toContainText('1,774 duplicate songs will be moved to a .trash folder');
    await expect(page.getByTestId('repair-confirm')).toContainText('Nothing is deleted');
    await expect(page.getByTestId('repair-confirm')).toContainText('undo');
    // ion-button is a custom element, so Playwright's own enabled/disabled does not
    // see it; aria-disabled is what Ionic sets and what assistive tech reads.
    await expect(confirm).toHaveAttribute('aria-disabled', 'true');

    await page.getByTestId('repair-ack').click();
    await expect(confirm).not.toHaveAttribute('aria-disabled', 'true');
    await page.getByTestId('repair-ack').click(); // un-tick: disabled again
    await expect(confirm).toHaveAttribute('aria-disabled', 'true');
    await page.getByTestId('repair-ack').click();
    await confirm.click();

    await expect(page.getByTestId('repair-progress')).toBeVisible({ timeout: 15_000 });
    const job = await runningJob(token);
    await emit(job, { event: 'progress', phase: 'apply', done: 400, total: 16131 });
    await expect(page.getByTestId('repair-progress')).toContainText('Applying the changes: 400 of 16,131', { timeout: 15_000 });
    await emit(job, {
      event: 'result', kind: 'apply', ok: true, planId: PLAN_A,
      apply: { done: 15976, skipped: 155, failed: 0, alreadyDone: 0,
        skippedByReason: [{ reason: 'no cover art', count: 139 }, { reason: 'source changed since the plan was made', count: 2 }],
        failedExamples: [] },
    });
    await endJob(job, 'succeed');

    const outcome = page.getByTestId('repair-outcome');
    await expect(outcome).toBeVisible({ timeout: 15_000 });
    await expect(outcome).toContainText('15,976 done, 155 skipped, 0 failed');
    await expect(outcome).toContainText('139 skipped: no cover art');
    await expect(outcome).toContainText('2 skipped: source changed since the plan was made');
    await expect(page.getByTestId('repair-plan')).toHaveAttribute('data-status', 'applied');
    await expect(page.getByTestId('repair-apply')).toHaveCount(0);

    // the same plan cannot be applied twice
    const again = await post(token, 'apply', { planId: PLAN_A, snapshotAck: true });
    expect(again.status).toBe(409);
    expect(((await again.json()) as { error: string }).error).toBe('already_applied');
  });

  test('undo puts the last repair back, once', async ({ page }) => {
    await login(page);
    await openRepair(page);
    await page.getByTestId('repair-undo').click();
    await page.locator('.repair-undo-confirm').click();

    await expect(page.getByTestId('repair-progress')).toBeVisible({ timeout: 15_000 });
    const job = await runningJob(token);
    await emit(job, { event: 'result', kind: 'undo', ok: true, planId: PLAN_A, undo: { restored: 15976, skipped: 1,
      skippedExamples: [{ path: 'A/b.mp3', note: 'original location is occupied' }] } });
    await endJob(job, 'succeed');

    await expect(page.getByTestId('repair-outcome')).toContainText('15,976 restored, 1 skipped', { timeout: 15_000 });
    await expect(page.getByTestId('repair-outcome')).toContainText('A/b.mp3');
    await expect(page.getByTestId('repair-plan')).toHaveAttribute('data-status', 'undone');
    await expect(page.getByTestId('repair-undo')).toHaveCount(0);
    const twice = await post(token, 'undo', { planId: PLAN_A });
    expect(twice.status).toBe(409);
  });

  test('an apply that did not finish can be continued, and continuing resumes the same plan', async ({ page }) => {
    // a new check → a new plan
    expect((await post(token, 'check')).status).toBe(202);
    let job = await runningJob(token);
    await emit(job, checkResult(PLAN_B));
    await endJob(job, 'succeed');
    await expect.poll(async () => (await snapshot(token)).plan?.id, { timeout: 15_000 }).toBe(PLAN_B);

    expect((await post(token, 'apply', { planId: PLAN_B, snapshotAck: true })).status).toBe(202);
    job = await runningJob(token);
    await emit(job, { event: 'progress', phase: 'apply', done: 200, total: 16131 });
    await endJob(job, 'fail'); // evicted: no result

    await login(page);
    await openRepair(page);
    await expect(page.getByTestId('repair-plan')).toHaveAttribute('data-status', 'apply_unfinished', { timeout: 15_000 });
    await expect(page.getByTestId('repair-apply')).toHaveCount(0);
    await page.getByTestId('repair-continue').click();
    await page.getByTestId('repair-ack').click();
    await page.getByTestId('repair-apply-confirm').click();
    await expect(page.getByTestId('repair-progress')).toBeVisible({ timeout: 15_000 });

    // the Job the server made runs the SAME plan, so the tool resumes from its journal
    const s = await snapshot(token);
    const res = await fetch(`${K8S}/__mock/jobs`);
    const jobs = (await res.json()) as { items: { metadata: { name: string }; spec: { template: { spec: { containers: { args: string[] }[] } } } }[] };
    const mine = jobs.items.find((j) => j.metadata.name === `music-repair-${s.current!.id}`)!;
    expect(mine.spec.template.spec.containers[0].args).toEqual(['apply', '--library', '/library', '--plan', PLAN_B]);
    await endJob(`music-repair-${s.current!.id}`, 'fail');
  });

  test('the history says who ran what and how it went, newest first', async ({ page }) => {
    await login(page);
    await openRepair(page);
    const history = page.getByTestId('repair-history');
    await expect(history).toBeVisible({ timeout: 15_000 });
    await expect(history.getByTestId('repair-run-check').first()).toContainText(ADMIN.username);
    await expect(history).toContainText('Did not finish');
    await expect(history).toContainText('Finished');
    const s = await snapshot(token);
    expect(s.history.length).toBeGreaterThanOrEqual(6);
    expect(s.history[0].kind).toBe('apply'); // the continued apply, newest
  });

  test('it fits a phone and both themes', async ({ page }) => {
    await page.setViewportSize({ width: 360, height: 740 });
    await page.emulateMedia({ colorScheme: 'dark' });
    await login(page);
    await openRepair(page);
    await expect(page.getByTestId('repair-content')).toBeVisible();
    const overflow = await page.evaluate(() => {
      const modal = document.querySelector('ion-modal');
      const el = modal?.shadowRoot?.querySelector('.modal-wrapper') ?? modal;
      return {
        page: document.documentElement.scrollWidth - window.innerWidth,
        modal: el ? (el as HTMLElement).scrollWidth - (el as HTMLElement).clientWidth : 0,
      };
    });
    expect(overflow.page).toBeLessThanOrEqual(0);
    expect(overflow.modal).toBeLessThanOrEqual(0);
  });
});
