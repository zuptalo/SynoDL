/**
 * The YouTube sign-in for download workers (spec 1055).
 *
 * The mock cluster holds Jobs and runs nothing, so the worker's side is played by
 * the test: it reads the Job the server created (which is exactly what anyone with
 * read access to Jobs could do) and tries to use what it finds. The point of the
 * whole feature is what is NOT in that Job, so most of this file is looking for
 * cookie values in places they must never be.
 *
 * Every cookie value carries MARKER, so a leak is a substring search.
 */
import { readdirSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { expect, test } from '@playwright/test';
import { ADMIN, apiToken, clearYtdl, createSecondUser, login } from './helpers';

const SF_PORT = Number(process.env.SYNODL_E2E_SF_PORT) || 8283;
const K8S = `http://localhost:${process.env.SYNODL_E2E_SF_K8S_PORT || 8296}`;
const API = `http://localhost:${SF_PORT}`;
const MARKER = 'SECRETVALUE-e2e-5d1f';

const HEADER =
  `Cookie: SAPISID=${MARKER}1; __Secure-3PSID=${MARKER}2; LOGIN_INFO=${MARKER}3; SID=${MARKER}4; YSC=${MARKER}5`;

test.describe.configure({ mode: 'serial' });

interface View {
  available: boolean;
  saved: boolean;
  cookieCount: number;
  loginCookies: string[];
  missingLogin: string[];
  warning?: string;
}

const auth = (token: string) => ({ 'Content-Type': 'application/json', 'X-SynoDL-Session': token });

async function put(token: string, text: string): Promise<Response> {
  return fetch(`${API}/v1/youtube/signin`, { method: 'PUT', headers: auth(token), body: JSON.stringify({ text }) });
}

interface MockJob {
  metadata: { name: string; labels?: Record<string, string> };
  spec: {
    template: {
      spec: {
        initContainers?: { name: string; image: string; command: string[]; env?: { name: string; value: string }[] }[];
        containers: { args: string[]; volumeMounts?: { name: string }[] }[];
        volumes?: { name: string; emptyDir?: { medium?: string; sizeLimit?: string }; persistentVolumeClaim?: unknown }[];
      };
    };
  };
}

async function jobs(): Promise<MockJob[]> {
  const res = await fetch(`${K8S}/__mock/jobs`);
  return ((await res.json()) as { items: MockJob[] }).items ?? [];
}

/** Submit a YouTube link and wait for the server to create its worker. */
async function download(token: string, url: string): Promise<{ requestId: string; job: MockJob }> {
  const res = await fetch(`${API}/v1/ytdl`, {
    method: 'POST',
    headers: auth(token),
    body: JSON.stringify({ url, mode: 'music' }),
  });
  expect(res.status).toBeLessThan(300);
  const { requestId } = (await res.json()) as { requestId: string };
  let job: MockJob | undefined;
  await expect
    .poll(async () => {
      job = (await jobs()).find((j) => j.metadata.name.includes(requestId.toLowerCase()));
      return Boolean(job);
    }, { timeout: 30_000 })
    .toBe(true);
  return { requestId, job: job as MockJob };
}

test.describe('YouTube sign-in (spec 1055)', () => {
  let token = '';

  test.beforeAll(async () => {
    token = await apiToken();
    await fetch(`${K8S}/__mock/reset`, { method: 'POST' });
    await clearYtdl(token);
    await fetch(`${API}/v1/youtube/signin`, { method: 'DELETE', headers: auth(token) });
  });

  test('only an admin has it', async () => {
    const user = await createSecondUser(token, 'signinbo');
    for (const [method, body] of [['GET', undefined], ['PUT', JSON.stringify({ text: 'a=1; b=2; c=3' })], ['DELETE', undefined]] as const) {
      const res = await fetch(`${API}/v1/youtube/signin`, { method, headers: auth(user), body });
      expect(res.status, `${method} as a regular user`).toBe(403);
    }
    const anon = await fetch(`${API}/v1/youtube/signin`);
    expect(anon.status).toBe(401);
  });

  test('a job started BEFORE anything is saved is anonymous', async () => {
    const { job } = await download(token, 'https://youtu.be/nosignin01');
    expect(job.metadata.labels?.['synodl.io/signin']).toBeUndefined();
    expect(job.spec.template.spec.initContainers ?? []).toHaveLength(0);
    expect(job.spec.template.spec.containers[0].args).not.toContain('--cookies');
  });

  test('saving shows counts and never a value', async () => {
    const res = await put(token, HEADER);
    expect(res.status).toBe(200);
    const raw = await res.text();
    expect(raw).not.toContain(MARKER);
    const v = JSON.parse(raw) as View;
    expect(v.saved).toBe(true);
    expect(v.cookieCount).toBe(5);
    expect(v.loginCookies.sort()).toEqual(['LOGIN_INFO', 'SAPISID', 'SID', '__Secure-3PSID']);
    expect(v.missingLogin).toEqual([]);

    const get = await (await fetch(`${API}/v1/youtube/signin`, { headers: auth(token) })).text();
    expect(get).not.toContain(MARKER);
  });

  test('junk is refused with a reason that does not repeat it', async () => {
    for (const [text, code] of [
      [`SAPISID=${MARKER}`, 'too_few_cookies'],
      [`just some words ${MARKER}`, 'unrecognised'],
    ] as const) {
      const res = await put(token, text);
      expect(res.status).toBe(400);
      const body = await res.text();
      expect(body).toContain(code);
      expect(body).not.toContain(MARKER);
    }
  });

  test('a job started AFTER saving carries a grant and no cookie value', async () => {
    const { job } = await download(token, 'https://youtu.be/withsignin1');
    const pod = job.spec.template.spec;
    expect(job.metadata.labels?.['synodl.io/signin']).toBe('true');

    const init = pod.initContainers ?? [];
    expect(init).toHaveLength(1);
    expect(init[0].name).toBe('signin');
    const grant = init[0].env?.find((e) => e.name === 'SYNODL_SIGNIN_GRANT')?.value ?? '';
    expect(grant).toMatch(/^[0-9a-f]{64}$/);
    expect(init[0].command.join(' ')).not.toContain(grant);

    const vol = pod.volumes?.find((v) => v.name === 'signin');
    expect(vol?.emptyDir?.medium).toBe('Memory');
    expect(vol?.emptyDir?.sizeLimit).toBeTruthy();
    expect(pod.volumes?.filter((v) => v.persistentVolumeClaim)).toHaveLength(1);
    const args = pod.containers[0].args;
    expect(args[args.indexOf('--cookies') + 1]).toBe('/signin/cookies.txt');

    // Anyone who can read Jobs sees all of this. None of it is a cookie.
    const everything = JSON.stringify(await jobs());
    expect(everything).not.toContain(MARKER);
    // The grant is worthless from here: it was not issued to this address, and it
    // carries no proxy header either way.
    for (const headers of [{}, { 'X-Forwarded-For': '10.42.0.97' }] as const) {
      const res = await fetch(`${API}/v1/internal/ytdl-signin`, {
        headers: { Authorization: `Bearer ${grant}`, ...headers },
      });
      expect(res.status).toBe(404);
      expect(await res.text()).toBe('');
    }
    const none = await fetch(`${API}/v1/internal/ytdl-signin`);
    expect(none.status).toBe(404);
  });

  test('the Settings screen: paste, save, and the paste is gone', async ({ page }) => {
    await login(page);
    await page.goto('/tabs/settings');
    await page.getByTestId('settings-youtube-signin').click();
    await expect(page.getByTestId('youtube-signin-modal')).toBeVisible();
    await expect(page.getByTestId('youtube-signin-status')).toBeVisible();

    const box = page.getByTestId('youtube-signin-text').locator('textarea');
    await box.fill(`Cookie: SAPISID=${MARKER}a; __Secure-3PSID=${MARKER}b; YSC=${MARKER}c; PREF=${MARKER}d`);
    await page.getByTestId('youtube-signin-save').click();
    await expect(page.getByTestId('youtube-signin-saved')).toBeVisible();

    // The value is nowhere on the page afterwards, and the field is empty.
    expect(await page.content()).not.toContain(MARKER);
    await expect(box).toHaveValue('');
    await expect(page.getByTestId('youtube-signin-status')).toContainText('4');
  });

  test('removing it leaves later jobs anonymous and nothing stored', async () => {
    const del = await fetch(`${API}/v1/youtube/signin`, { method: 'DELETE', headers: auth(token) });
    expect(del.status).toBe(204);
    const v = (await (await fetch(`${API}/v1/youtube/signin`, { headers: auth(token) })).json()) as View;
    expect(v.saved).toBe(false);
    const { job } = await download(token, 'https://youtu.be/afterremove');
    expect(job.metadata.labels?.['synodl.io/signin']).toBeUndefined();
    expect(job.spec.template.spec.initContainers ?? []).toHaveLength(0);
  });

  test('no cookie value reached the server log or the database files', async () => {
    // Everything above pasted markers through the real server. It logs to a file
    // and stores to a directory in this run; neither may hold one.
    const log = readFileSync(path.join(process.cwd(), '.tmp', 'synodl-e2e-sf.log'), 'utf8');
    expect(log, 'the server log contains a cookie value').not.toContain(MARKER);
    const dir = path.join(process.cwd(), '.tmp', 'e2e-sf-data');
    for (const f of readdirSync(dir)) {
      const bytes = readFileSync(path.join(dir, f));
      expect(bytes.includes(MARKER), `${f} holds a cookie value in the clear`).toBe(false);
    }
    void ADMIN;
  });
});
