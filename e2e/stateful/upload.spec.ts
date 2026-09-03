/**
 * Uploading a file straight into the library (spec 1022).
 *
 * This is the one route that puts file CONTENT on the NAS, so the coverage here
 * is about the boundary as much as the happy path: where a file may land, and
 * that a client cannot talk it into landing anywhere else.
 */
import { expect, test } from '@playwright/test';
import { addSource, apiToken, clearSources, login, setSourceState } from './helpers';

const API = `http://localhost:${Number(process.env.SYNODL_E2E_SF_PORT) || 8283}`;

let token = '';

test.beforeEach(async () => {
  token = await apiToken();
  await clearSources(token);
  await setSourceState('reset');
  await addSource(token, 'Only Source', 0);
});

/** POST a multipart upload directly, so the boundary can be probed. */
async function upload(fields: Record<string, string>, filename: string, body = 'x') {
  const form = new FormData();
  for (const [k, v] of Object.entries(fields)) form.append(k, v);
  // The real client sends the byte count ahead of the file so the server can give
  // the NAS an exact Content-Length (DSM refuses a chunked upload body). Cases
  // that deliberately omit it pass their own `size` in `fields`.
  if (!('size' in fields)) form.append('size', String(new Blob([body]).size));
  form.append('file', new Blob([body]), filename);
  const res = await fetch(`${API}/v1/fs/upload`, {
    method: 'POST',
    headers: { 'X-SynoDL-Session': token },
    body: form,
  });
  return { status: res.status, body: (await res.json().catch(() => ({}))) as Record<string, string> };
}

test('a movie is named the way a download would name it', async () => {
  const r = await upload({ kind: 'movie', title: 'Dune 2021' }, 'Dune.2021.mkv');
  expect(r.status).toBe(200);
  expect(r.body.destination).toBe('movie/Dune (2021)');
});

test('an episode lands in its season folder', async () => {
  const r = await upload({ kind: 'tv', title: 'Friends 1994 - 2004', season: '1' }, 'Friends.S01E01.mkv');
  expect(r.status).toBe(200);
  expect(r.body.destination).toBe('tv-show/Friends (1994)/Season 01');
});

test('a second episode joins the same show and season', async () => {
  await upload({ kind: 'tv', title: 'Friends (1994)', season: '2' }, 'Friends.S02E01.mkv');
  const r = await upload({ kind: 'tv', title: 'Friends (1994)', season: '2' }, 'Friends.S02E02.mkv');
  expect(r.status).toBe(200);
  expect(r.body.destination).toBe('tv-show/Friends (1994)/Season 02');
});

test('the same file twice is refused, never overwritten', async () => {
  await upload({ kind: 'movie', title: 'Dune 2021' }, 'Dune.mkv', 'first');
  const r = await upload({ kind: 'movie', title: 'Dune 2021' }, 'Dune.mkv', 'second');
  expect(r.status).toBe(409);
  expect(r.body.error).toBe('file_exists');
});

test('only media and sidecar files are accepted', async () => {
  for (const name of ['payload.sh', 'tool.exe', 'archive.zip']) {
    expect((await upload({ kind: 'movie', title: 'X 2020' }, name)).status).toBe(415);
  }
  for (const name of ['a.mkv', 'a.srt', 'a.jpg', 'a.nfo']) {
    expect((await upload({ kind: 'movie', title: 'X 2020' }, name)).status).toBe(200);
  }
});

test('a file cannot be placed outside the configured parents', async () => {
  // No request shape names a path: the parent is one of two words.
  for (const kind of ['home', '/home', '../home', 'music', '']) {
    expect((await upload({ kind, title: 'X 2020' }, 'a.mkv')).status).toBe(409);
  }
  // A hostile file name never escapes the folder the server composed.
  const r = await upload({ kind: 'movie', title: 'X 2020' }, '../../../etc/passwd.mkv');
  expect(r.status).toBe(200);
  expect(r.body.destination).toBe('movie/X (2020)');
  expect(r.body.file).toBe('passwd.mkv');
});

test('a title is required', async () => {
  expect((await upload({ kind: 'movie', title: '' }, 'a.mkv')).status).toBe(400);
  expect((await upload({ kind: 'movie', title: '...' }, 'a.mkv')).status).toBe(400);
});

test('the upload flow works from the Tasks tab', async ({ page }) => {
  await login(page);
  await page.goto('/tabs/tasks');

  await page.getByTestId('newtask-fab').click();
  await page.getByTestId('upload-open').click();

  await page.getByTestId('upload-title').locator('input').fill('Arrival 2016');
  await page.getByTestId('upload-input').setInputFiles({
    name: 'Arrival.2016.mkv',
    mimeType: 'video/x-matroska',
    buffer: Buffer.from('a short film'),
  });

  // The destination is shown before committing to it, named the way the SERVER
  // will name it. The preview used to show the raw title ("movie/Arrival 2016")
  // and so promised a folder the file never landed in.
  await expect(page.getByTestId('upload-preview')).toContainText('movie/Arrival (2016)');

  await page.getByTestId('upload-send').click();

  // The row reports where the file actually landed, which is the outcome that
  // matters — and it now matches what the preview promised.
  // Scoped to the row rather than the page: the preview now says the same thing,
  // which is the point, so a bare text match would find both.
  await expect(page.getByTestId('upload-result')).toContainText('movie/Arrival (2016)', {
    timeout: 30_000,
  });
});


test('an upload without its byte count is refused', async () => {
  // The server cannot build a NAS request without knowing the length up front,
  // and guessing is not an option — so this fails loudly rather than falling
  // back to a chunked body the NAS would reject anyway.
  const form = new FormData();
  form.append('kind', 'movie');
  form.append('title', 'Sizeless 2021');
  form.append('file', new Blob(['x']), 'a.mkv');
  const res = await fetch(`${API}/v1/fs/upload`, {
    method: 'POST',
    headers: { 'X-SynoDL-Session': token },
    body: form,
  });
  expect(res.status).toBe(400);
});

test('replacing is opt-in, and is what recovers a partial file', async () => {
  const f = 'Partial.2021.mkv';
  expect((await upload({ kind: 'movie', title: 'Partial 2021' }, f, 'half a file')).status).toBe(200);

  // Without asking, the name is defended — nothing is destroyed by accident.
  const clash = await upload({ kind: 'movie', title: 'Partial 2021' }, f, 'the whole file');
  expect(clash.status).toBe(409);
  expect(clash.body.error).toBe('file_exists');

  // An interrupted upload leaves a fragment behind, and this is the only way
  // past it: the user explicitly chooses to replace.
  const replaced = await upload(
    { kind: 'movie', title: 'Partial 2021', overwrite: 'true' },
    f,
    'the whole file',
  );
  expect(replaced.status).toBe(200);
  expect(replaced.body.destination).toBe('movie/Partial (2021)');
});
