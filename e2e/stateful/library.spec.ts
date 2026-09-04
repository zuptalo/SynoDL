/**
 * Discover marks what you already have (spec 0008, US1).
 *
 * The fake source's titles are generated, so rather than hard-coding one, each
 * spec asks the API what the catalog is actually serving and seeds a folder for
 * exactly that title. That also exercises the property the whole feature rests
 * on: the folder SynoDL creates is named after the catalog title, so the two
 * sides really do line up.
 *
 * Note the seeding order. The server holds its reading of the NAS for five
 * minutes, so any spec that seeds after a search has already warmed the snapshot
 * must call refreshLibrary() — the same invalidation a configuration change or a
 * send triggers for a real user.
 */
import { expect, test } from '@playwright/test';
import {
  addSource,
  apiSearch,
  apiTitle,
  apiToken,
  clearSources,
  folderNameFor,
  gotoDiscover,
  login,
  refreshLibrary,
  seedLibrary,
  seedLibraryFiles,
  setHideOwned,
  setSourceState,
} from './helpers';

let token = '';

test.beforeEach(async () => {
  token = await apiToken();
  await clearSources(token);
  await setSourceState('reset');
  await seedLibrary({}); // restore the fixture tree; seeding is per-spec
});

const parentFor = (type: string) => (type === 'series' || type === 'anime' ? '/tv-show' : '/movie');

test('a title already on the NAS is marked, and its neighbours are not', async ({ page }) => {
  const id = await addSource(token, 'Only Source', 0);

  const items = await apiSearch(token);
  expect(items.length).toBeGreaterThan(1);
  const owned = items[0];
  const folder = folderNameFor(owned.title);
  await seedLibrary({ [parentFor(owned.type)]: [folder] });
  // The folder alone is not evidence — the video has to actually be there.
  await seedLibraryFiles({ [`${parentFor(owned.type)}/${folder}`]: ['episode.mkv'] });
  await refreshLibrary(token, id, 'Only Source');

  // FR-001: the API now reports exactly that one title as present.
  const after = await apiSearch(token);
  expect(after.find((i) => i.title === owned.title)?.ownership).toBe('owned');
  expect(after.filter((i) => i.ownership === 'owned').length).toBe(1);

  await login(page);
  await gotoDiscover(page);

  // FR-011/FR-012: one card carries the marker, and it is announced rather than
  // conveyed by colour alone.
  const marker = page.locator('.ribbon');
  await expect(marker.first()).toBeVisible();
  await expect(marker).toHaveCount(1);
  await expect(marker.first()).toHaveAttribute('aria-label', /already in your library/i);
});

test('nothing is marked when the NAS holds none of the catalog', async ({ page }) => {
  await addSource(token, 'Only Source', 0);
  await login(page);
  await gotoDiscover(page);

  await expect(page.locator('.card').first()).toBeVisible();
  await expect(page.locator('.ribbon')).toHaveCount(0);
});

// SC-004: matching must not be loose. A folder that merely resembles a catalog
// title must not mark it — a false positive makes a user skip something they
// wanted. (The release-year half of this rule is exercised directly in
// internal/library and in the handler tests, where a year can actually be varied;
// the fake source's titles carry none.)
test('a near-miss folder name does not mark a title', async ({ page }) => {
  const id = await addSource(token, 'Only Source', 0);

  const items = await apiSearch(token);
  const subject = items[0];
  await seedLibrary({ [parentFor(subject.type)]: [`${folderNameFor(subject.title)} Behind The Scenes`] });
  await refreshLibrary(token, id, 'Only Source');

  const after = await apiSearch(token);
  expect(after.find((i) => i.title === subject.title)?.ownership ?? 'absent').not.toBe('owned');

  await login(page);
  await gotoDiscover(page);
  await expect(page.locator('.card').first()).toBeVisible();
  await expect(page.locator('.ribbon')).toHaveCount(0);
});

// FR-009: browsing must survive parent folders that cannot be read — no marker,
// no error, nothing the user has to think about.
test('Discover still works when the parent folders cannot be read', async ({ page }) => {
  const id = await addSource(token, 'Only Source', 0);
  const API = `http://localhost:${Number(process.env.SYNODL_E2E_SF_PORT) || 8283}`;
  const res = await fetch(`${API}/v1/source/providers/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json', 'X-SynoDL-Session': token },
    body: JSON.stringify({
      kind: 'zarfilm',
      displayName: 'Only Source',
      moviesParent: 'no-such-parent',
      tvParent: 'no-such-parent-either',
      session: {},
    }),
  });
  expect(res.ok).toBe(true);

  await login(page);
  await gotoDiscover(page);

  await expect(page.locator('.card').first()).toBeVisible();
  await expect(page.locator('.ribbon')).toHaveCount(0);
  // The failed scan is invisible: no empty/error state took over the grid.
  await expect(page.locator('.state')).toHaveCount(0);
});


// The defect this amendment corrects. A folder full of artwork and metadata is
// not content, and 0.3.0 marked exactly that as owned — the operator's NAS had
// "Attack on Titan (2013)/Season 00" holding nothing but season.nfo.
test('a folder holding only metadata is not owned', async () => {
  const id = await addSource(token, 'Only Source', 0);
  const items = await apiSearch(token);
  const subject = items[0];
  const folder = folderNameFor(subject.title);

  await seedLibrary({ [parentFor(subject.type)]: [folder] });
  await seedLibraryFiles({
    [`${parentFor(subject.type)}/${folder}`]: ['season.nfo', 'poster.jpg', 'subs.srt'],
  });
  await refreshLibrary(token, id, 'Only Source');

  const after = await apiSearch(token);
  expect(after.find((i) => i.title === subject.title)?.ownership).toBe('absent');
  expect(after.filter((i) => i.ownership === 'owned').length).toBe(0);
});

// FR-015: a series keeps its episodes one level down, and that is still content.
test('video in a season subfolder counts as owned', async () => {
  const id = await addSource(token, 'Only Source', 0);
  const items = await apiSearch(token);
  // Any type will do here: the point is that video one level DOWN still counts,
  // which is true for a movie folder with a subfolder as much as for a series.
  const subject = items[0];
  const folder = folderNameFor(subject.title);
  const base = `${parentFor(subject.type)}/${folder}`;

  await seedLibrary({ [parentFor(subject.type)]: [folder], [base]: ['Season 01'] });
  await seedLibraryFiles({
    [base]: ['poster.jpg'],
    [`${base}/Season 01`]: ['Show.S01E01.mkv'],
  });
  await refreshLibrary(token, id, 'Only Source');

  const after = await apiSearch(token);
  expect(after.find((i) => i.title === subject.title)?.ownership).toBe('owned');
});


// US2: which seasons are here, and which episodes each holds. The gap is what
// makes it actionable — a user with episodes 1, 2 and 6 should see exactly that.
test('a series reports which seasons and episodes are already here', async () => {
  const id = await addSource(token, 'Only Source', 0);
  const items = await apiSearch(token);
  const series = items.find((i) => i.type === 'series' || i.type === 'anime');
  // Skip rather than fall back to a movie: a movie has no season breakdown by
  // design, so substituting one would assert series behaviour against something
  // that is right to lack it, and pass or fail for the wrong reason.
  test.skip(!series, 'catalog served no series');
  const folder = folderNameFor(series!.title);
  const base = `${parentFor(series!.type)}/${folder}`;

  await seedLibrary({
    [parentFor(series!.type)]: [folder],
    [base]: ['Season 01', 'Season 02', 'Specials'],
  });
  await seedLibraryFiles({
    [`${base}/Season 01`]: ['S.S01E01.mkv', 'S.S01E02.mkv', 'S.S01E06.mkv'],
    [`${base}/Season 02`]: ['S.S02E01.mkv'],
    [`${base}/Specials`]: ['season.nfo'], // metadata only — not present
  });
  await refreshLibrary(token, id, 'Only Source');

  const detail = await apiTitle(token, series!.id);
  expect(detail.ownership).toBe('owned');

  const bySeason = new Map((detail.seasons ?? []).map((s) => [s.season, s]));
  expect(bySeason.get(1)?.episodes).toEqual([1, 2, 6]);
  expect(bySeason.get(2)?.episodes).toEqual([1]);
  // A season folder holding only metadata is not present, just as a title folder
  // holding only metadata is not owned.
  expect(bySeason.has(0)).toBe(false);

  // FR-016a: nothing may claim a season is complete or state a total. The
  // catalog's episode count is not reliable, so the shape must not imply one.
  const raw = JSON.stringify(detail);
  expect(raw).not.toContain('"total"');
  expect(raw).not.toContain('"complete"');
});

// FR-018: a movie is owned without a season breakdown.
test('a movie reports no seasons', async () => {
  const id = await addSource(token, 'Only Source', 0);
  const items = await apiSearch(token);
  const movie = items.find((i) => i.type === 'movie');
  test.skip(!movie, 'catalog served no movie');
  const folder = folderNameFor(movie!.title);

  await seedLibrary({ [parentFor(movie!.type)]: [folder] });
  await seedLibraryFiles({ [`${parentFor(movie!.type)}/${folder}`]: ['film.mkv'] });
  await refreshLibrary(token, id, 'Only Source');

  const detail = await apiTitle(token, movie!.id);
  expect(detail.ownership).toBe('owned');
  expect(detail.seasons ?? []).toEqual([]);
});

// US3: hiding covers what you HAVE and what is already on its way — neither is
// something to send again, and a downloading title's progress belongs in Tasks.
//
// The toggle is set through the API rather than the filter sheet. The sheet is an
// Ionic modal whose Options section sits below the fold and keeps animating past
// Playwright's stability window, and driving it proved flaky without testing
// anything the unit tests do not already cover: the toggle only flips a boolean.
// What matters end to end is that a STORED preference reaches the grid and
// survives a reload (FR-022, FR-024), which is what this asserts.
test('a stored hide-owned preference removes owned titles and survives a reload', async ({
  page,
}) => {
  const id = await addSource(token, 'Only Source', 0);
  const items = await apiSearch(token);
  expect(items.length).toBeGreaterThan(1);
  const owned = items[0];
  const folder = folderNameFor(owned.title);

  await seedLibrary({ [parentFor(owned.type)]: [folder] });
  await seedLibraryFiles({ [`${parentFor(owned.type)}/${folder}`]: ['film.mkv'] });
  await refreshLibrary(token, id, 'Only Source');
  expect((await apiSearch(token)).find((i) => i.title === owned.title)?.ownership).toBe('owned');

  await login(page);
  await gotoDiscover(page);
  // Visible first, so the disappearance below means something.
  await expect(page.locator('.ribbon').first()).toBeVisible();

  await setHideOwned(token, true);
  await page.reload();
  await gotoDiscover(page);

  // The owned card is gone, and the grid still has results — hiding must not
  // empty the view (FR-023a).
  await expect(page.locator('.ribbon')).toHaveCount(0);
  await expect(page.getByTestId('catalog-card').first()).toBeVisible();
});
