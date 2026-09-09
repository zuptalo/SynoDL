/**
 * Uploading a track (spec 1040).
 *
 * The interesting coverage, as with spec 1022's uploads, is the boundary: the
 * SERVER composes the destination, three more client strings reach a path
 * segment, and what a determined value can do to that path is the point.
 */
import { expect, test } from '@playwright/test';
import { addSource, apiToken, clearSources, login, setSourceState } from './helpers';

const API = `http://localhost:${Number(process.env.SYNODL_E2E_SF_PORT) || 8283}`;

let token = '';

test.beforeEach(async () => {
  token = await apiToken();
  // A download source gives the sheet its film and TV parents, so the tests
  // below are about MUSIC being configured or not rather than about an instance
  // with no libraries at all.
  await clearSources(token);
  await setSourceState('reset');
  await addSource(token, 'Only Source', 0);
  await setLibraries('music', 'music-video');
});

async function setLibraries(music: string, musicVideo: string): Promise<void> {
  const res = await fetch(`${API}/v1/library/music`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json', 'X-SynoDL-Session': token },
    body: JSON.stringify({ music, musicVideo }),
  });
  expect(res.status).toBe(200);
}

async function uploadTrack(
  fields: Record<string, string>,
  filename: string,
  body = 'x',
): Promise<{ status: number; body: Record<string, string> }> {
  const form = new FormData();
  for (const [k, v] of Object.entries(fields)) form.append(k, v);
  form.append('size', String(new Blob([body]).size));
  form.append('file', new Blob([body]), filename);
  const res = await fetch(`${API}/v1/fs/upload`, {
    method: 'POST',
    headers: { 'X-SynoDL-Session': token },
    body: form,
  });
  return { status: res.status, body: (await res.json().catch(() => ({}))) as Record<string, string> };
}

test('a track is filed the way a downloaded one is', async () => {
  const r = await uploadTrack(
    { kind: 'music', track: 'Lucente', artist: 'Anyma', album: 'The End Of Genesys' },
    'whatever-the-phone-called-it.mp3',
  );
  expect(r.status).toBe(200);
  expect(r.body.destination).toBe('music/Anyma/The End Of Genesys');
  expect(r.body.file).toBe('Lucente.mp3');
});

test('a track with no album is filed under Singles, like a download with none', async () => {
  const r = await uploadTrack({ kind: 'music', track: 'Sonder', artist: 'Anyma' }, 'a.mp3');
  expect(r.status).toBe(200);
  expect(r.body.destination).toBe('music/Anyma/Singles');
});

test('lyrics take the track name, so a media server pairs them with the audio', async () => {
  const audio = await uploadTrack(
    { kind: 'music', track: 'Lucente', artist: 'Anyma', album: 'Genesys' },
    'a.mp3',
  );
  const lyrics = await uploadTrack(
    { kind: 'music', track: 'Lucente', artist: 'Anyma', album: 'Genesys' },
    'downloaded-words.lrc',
  );
  expect(audio.body.file).toBe('Lucente.mp3');
  expect(lyrics.body.file).toBe('Lucente.lrc');
  expect(lyrics.body.destination).toBe(audio.body.destination);
});

test('a thumbnail becomes the album cover, not a stray image', async () => {
  const r = await uploadTrack(
    { kind: 'music', track: 'Lucente', artist: 'Anyma', album: 'Genesys' },
    'IMG_9931.JPG',
  );
  expect(r.status).toBe(200);
  expect(r.body.file).toBe('cover.jpg');
  expect(r.body.destination).toBe('music/Anyma/Genesys');
});

test('a music video goes to its own library', async () => {
  const r = await uploadTrack(
    { kind: 'music-video', track: 'Lucente', artist: 'Anyma', album: 'Genesys' },
    'clip.mp4',
  );
  expect(r.status).toBe(200);
  expect(r.body.destination).toBe('music-video/Anyma/Genesys');
  expect(r.body.file).toBe('Lucente.mp4');
});

test('nothing typed into the fields can write outside the library', async () => {
  // A bare ".." is the one that matters: it holds no separator to replace, so
  // most cleaning leaves it exactly as it was.
  for (const artist of ['..', '.', '...', '   ', '/']) {
    const r = await uploadTrack({ kind: 'music', track: 'T', artist }, 'a.mp3');
    expect(r.status, `artist ${JSON.stringify(artist)}`).toBe(400);
  }
  for (const track of ['..', '.', '  ']) {
    const r = await uploadTrack({ kind: 'music', track, artist: 'Anyma' }, 'a.mp3');
    expect(r.status, `track ${JSON.stringify(track)}`).toBe(400);
  }
  // A separator inside a real name is neutered rather than refused: AC/DC is an
  // artist, not an attack, and it must stay inside one folder.
  const ok = await uploadTrack({ kind: 'music', track: 'Back In Black', artist: 'AC/DC' }, 'a.mp3');
  expect(ok.status).toBe(200);
  expect(ok.body.destination).toBe('music/AC DC/Singles');
});

test('each library only accepts what it is for', async () => {
  const video = await uploadTrack({ kind: 'music', track: 'T', artist: 'A' }, 'clip.mkv');
  expect(video.status).toBe(415);
  const audio = await uploadTrack({ kind: 'music-video', track: 'T', artist: 'A' }, 'track.mp3');
  expect(audio.status).toBe(415);
});

test('a library that is not configured is refused rather than half-working', async () => {
  await setLibraries('', '');
  const r = await uploadTrack({ kind: 'music', track: 'T', artist: 'A' }, 'a.mp3');
  expect(r.status).toBe(409);
  expect(r.body.error).toBe('parent_unset');
});

test('the upload sheet offers Music only once a library is configured', async ({ page }) => {
  await setLibraries('', '');
  await login(page);
  await page.goto('/tabs/tasks');
  await page.getByTestId('newtask-fab').click();
  await page.getByTestId('upload-open').click();
  // Films are configured, so the sheet is usable — it simply does not offer an
  // option that cannot work.
  await expect(page.getByTestId('upload-kind-movie')).toBeVisible();
  await expect(page.getByTestId('upload-kind-music')).toHaveCount(0);
  await expect(page.getByTestId('upload-kind-music-video')).toHaveCount(0);

  await setLibraries('music', 'music-video');
  await page.reload();
  await page.getByTestId('newtask-fab').click();
  await page.getByTestId('upload-open').click();
  await expect(page.getByTestId('upload-kind-music')).toBeVisible();
  await expect(page.getByTestId('upload-kind-music-video')).toBeVisible();
});

test('the sheet asks for a track, an artist and an album, and previews where it lands', async ({
  page,
}) => {
  await login(page);
  await page.goto('/tabs/tasks');
  await page.getByTestId('newtask-fab').click();
  await page.getByTestId('upload-open').click();
  await page.getByTestId('upload-kind-music').click();

  await page.getByTestId('upload-track').locator('input').fill('Lucente');
  await page.getByTestId('upload-artist').locator('input').fill('Anyma');
  await expect(page.getByTestId('upload-preview')).toContainText('music/Anyma/Singles/Lucente');

  await page.getByTestId('upload-album').locator('input').fill('The End Of Genesys');
  await expect(page.getByTestId('upload-preview')).toContainText(
    'music/Anyma/The End Of Genesys/Lucente',
  );
});
