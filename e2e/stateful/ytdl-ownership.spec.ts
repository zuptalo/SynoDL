/**
 * Who can see whose YouTube downloads (spec 0013, US1).
 *
 * This closes a defect rather than adding a feature: before spec 0013 the list
 * returned every user's YouTube downloads to every signed-in user, and gated
 * only the "added by" name behind admin. The rule is now the one NAS tasks
 * already follow — your own, unless you are an admin.
 *
 * Driven through the API rather than the UI, because the thing under test is
 * what the server is willing to say, and two browser sessions would only make
 * that harder to read.
 */
import { expect, test } from '@playwright/test';
import { ADMIN, apiToken } from './helpers';

const SF_PORT = Number(process.env.SYNODL_E2E_SF_PORT) || 8283;
const K8S = `http://localhost:${process.env.SYNODL_E2E_SF_K8S_PORT || 8296}`;
const API = `http://localhost:${SF_PORT}`;

type Download = { requestId: string; url: string; submittedBy?: string };

async function submitAs(token: string, url: string): Promise<string> {
  const res = await fetch(`${API}/v1/ytdl`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-SynoDL-Session': token },
    body: JSON.stringify({ url, mode: 'music' }),
  });
  expect(res.status, `submit ${url}`).toBe(202);
  return ((await res.json()) as { requestId: string }).requestId;
}

async function listAs(token: string): Promise<Download[]> {
  const res = await fetch(`${API}/v1/ytdl`, { headers: { 'X-SynoDL-Session': token } });
  expect(res.ok).toBe(true);
  return ((await res.json()) as { downloads: Download[] }).downloads;
}

/** Create a non-admin and sign them in, returning their token. */
async function secondUser(adminToken: string, username: string): Promise<string> {
  const password = `e2e-${username}-password`;
  const created = await fetch(`${API}/v1/users`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-SynoDL-Session': adminToken },
    body: JSON.stringify({ username, password, isAdmin: false }),
  });
  expect([200, 201], `create ${username}`).toContain(created.status);

  const session = await fetch(`${API}/v1/session`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  });
  expect(session.ok, `sign in ${username}`).toBe(true);
  return ((await session.json()) as { token: string }).token;
}

test.beforeEach(async () => {
  await fetch(`${K8S}/__mock/reset`, { method: 'POST' });
});

test('a user sees their own YouTube downloads and nobody else’s', async () => {
  const admin = await apiToken();
  const bo = await secondUser(admin, 'e2ebo');

  await submitAs(admin, 'https://youtu.be/adminOwnedSong');
  await submitAs(bo, 'https://youtu.be/boOwnedSong');

  const boSees = await listAs(bo);
  expect(boSees).toHaveLength(1);
  expect(boSees[0].url).toContain('boOwnedSong');

  // And attribution stays an admin-only field.
  expect(boSees[0].submittedBy ?? '').toBe('');
});

test('an admin sees everyone’s, each attributed', async () => {
  const admin = await apiToken();
  const bo = await secondUser(admin, 'e2ebo2');

  await submitAs(admin, 'https://youtu.be/adminOwnedSong');
  await submitAs(bo, 'https://youtu.be/boOwnedSong');

  const adminSees = await listAs(admin);
  expect(adminSees).toHaveLength(2);
  for (const d of adminSees) {
    expect(d.submittedBy, `${d.url} should be attributed for an admin`).toBeTruthy();
  }
  expect(adminSees.map((d) => d.submittedBy)).toContain(ADMIN.username);
});

test('another user’s download answers exactly as one that does not exist', async () => {
  // 404 rather than 403: a 403 would confirm the download exists, which is the
  // disclosure the rule is about.
  const admin = await apiToken();
  const bo = await secondUser(admin, 'e2ebo3');

  const requestId = await submitAs(admin, 'https://youtu.be/adminOwnedSong');

  const theirs = await fetch(`${API}/v1/ytdl/${requestId}`, {
    method: 'DELETE',
    headers: { 'X-SynoDL-Session': bo },
  });
  const imaginary = await fetch(`${API}/v1/ytdl/no-such-request`, {
    method: 'DELETE',
    headers: { 'X-SynoDL-Session': bo },
  });

  expect(theirs.status).toBe(404);
  expect(imaginary.status).toBe(theirs.status);

  // The admin's download survived the attempt.
  expect(await listAs(admin)).toHaveLength(1);
});
