/**
 * Which version you already have, and getting to the season you want (spec 1025).
 *
 * Ownership used to be answered per season, so every option for a season on the
 * NAS was stamped "Have it" — true of one of them and false of the rest. These
 * specs seed a KNOWN release and check that exactly the matching option is
 * marked, then that the options list opens on the first season the user is
 * missing rather than all of them at once.
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
  setSourceState,
} from './helpers';

let token = '';
const parentFor = (type: string) => (type === 'series' || type === 'anime' ? '/tv-show' : '/movie');

test.beforeEach(async () => {
  token = await apiToken();
  await clearSources(token);
  await setSourceState('reset');
});

test('only the release actually on the NAS is marked', async () => {
  const id = await addSource(token, 'Only Source', 0);
  const items = await apiSearch(token);
  const movie = items.find((i) => i.type === 'movie');
  test.skip(!movie, 'catalog served no movie');

  const folder = folderNameFor(movie!.title);
  const base = `${parentFor(movie!.type)}/${folder}`;
  await seedLibrary({ [parentFor(movie!.type)]: [folder] });
  // A real release name: the 1080p encode by this source's encoder.
  await seedLibraryFiles({ [base]: ['Mock.2024.1080p.BluRay-MockEnc.mkv'] });
  await refreshLibrary(token, id, 'Only Source');

  const detail = await apiTitle(token, movie!.id);
  const opts = detail.qualities ?? [];
  expect(opts.length).toBeGreaterThan(1);

  const marked = opts.filter((q) => q.owned);
  expect(marked.length, 'the release on disk is marked').toBeGreaterThan(0);
  // Every marked option is the release that is actually there. Two options that
  // agree on BOTH resolution and encoder are indistinguishable from a file name,
  // so both may be marked — what must never happen is marking an option the file
  // contradicts.
  for (const q of marked) {
    expect(q.resolution).toBe('1080p');
    expect((q.encoder ?? '').toLowerCase()).toContain('mockenc');
  }
  // The other resolutions this source offers must not be marked — that was the
  // bug: anything on disk stamped every option for the title.
  expect(opts.some((o) => o.resolution !== '1080p')).toBe(true);
  for (const q of opts.filter((o) => o.resolution !== '1080p')) {
    expect(q.owned, `${q.label} should not be marked`).toBeFalsy();
  }
});

test('files that do not name a release mark nothing, but the title is still owned', async () => {
  const id = await addSource(token, 'Only Source', 0);
  const items = await apiSearch(token);
  const movie = items.find((i) => i.type === 'movie');
  test.skip(!movie, 'catalog served no movie');

  const folder = folderNameFor(movie!.title);
  await seedLibrary({ [parentFor(movie!.type)]: [folder] });
  await seedLibraryFiles({ [`${parentFor(movie!.type)}/${folder}`]: ['the movie.mkv'] });
  await refreshLibrary(token, id, 'Only Source');

  const detail = await apiTitle(token, movie!.id);
  expect(detail.ownership).toBe('owned');
  expect((detail.qualities ?? []).some((q) => q.owned)).toBe(false);
});

test('a series opens on the first season you do not have, and one at a time', async ({ page }) => {
  const id = await addSource(token, 'Only Source', 0);
  const items = await apiSearch(token);
  const series = items.find((i) => i.type === 'series' || i.type === 'anime');
  test.skip(!series, 'catalog served no series');

  const folder = folderNameFor(series!.title);
  const base = `${parentFor(series!.type)}/${folder}`;
  // Seasons 1 and 2 are here; 3 is not.
  await seedLibrary({ [parentFor(series!.type)]: [folder], [base]: ['Season 01', 'Season 02'] });
  await seedLibraryFiles({
    [`${base}/Season 01`]: ['S.S01E01.mkv'],
    [`${base}/Season 02`]: ['S.S02E01.mkv'],
  });
  await refreshLibrary(token, id, 'Only Source');

  await login(page);
  await gotoDiscover(page);
  await page.getByTestId('catalog-card').filter({ hasText: series!.title }).first().click();

  const groups = page.getByTestId('season-group');
  await expect(groups).toHaveCount(3);

  // Which group is open is read off the accordion group's own value rather than
  // Ionic's internal classes. Because the group is single-open, that value is one
  // season — so this asserts "exactly one is open" and "which one" at once.
  const openSeason = () =>
    page.locator('ion-accordion-group.season-groups').evaluate((el) => (el as unknown as { value: string }).value);

  // FR-008: the first season NOT on the NAS is the one already open, so the
  // common case takes no taps.
  await expect.poll(openSeason, { timeout: 10_000 }).toBe('3');

  // FR-009: opening another closes it.
  await groups.first().click();
  await expect.poll(openSeason, { timeout: 10_000 }).toBe('1');

  // FR-010: a collapsed header still says whether that season is here.
  await expect(groups.nth(1)).toContainText('On your NAS');
});

// FR-008, the other half: with nothing left to fetch there is nothing to open on.
test('a series you already have opens fully collapsed', async ({ page }) => {
  const id = await addSource(token, 'Only Source', 0);
  const items = await apiSearch(token);
  const series = items.find((i) => i.type === 'series' || i.type === 'anime');
  test.skip(!series, 'catalog served no series');

  const folder = folderNameFor(series!.title);
  const base = `${parentFor(series!.type)}/${folder}`;
  const seasons = ['Season 01', 'Season 02', 'Season 03'];
  await seedLibrary({ [parentFor(series!.type)]: [folder], [base]: seasons });
  await seedLibraryFiles(
    Object.fromEntries(
      seasons.map((s, i) => [`${base}/${s}`, [`S.S0${i + 1}E01.mkv`]]),
    ),
  );
  await refreshLibrary(token, id, 'Only Source');

  await login(page);
  await gotoDiscover(page);
  await page.getByTestId('catalog-card').filter({ hasText: series!.title }).first().click();

  await expect(page.getByTestId('season-group')).toHaveCount(3);
  const open = await page
    .locator('ion-accordion-group.season-groups')
    .evaluate((el) => (el as unknown as { value: string | null }).value);
  expect(open ?? null, 'nothing to fetch, so nothing is opened').toBeFalsy();
});

// FR-013 / SC-005: the reported device clipped the badge and cut the episode list
// off mid-line. Nothing may overflow at 360px.
test('every row fits a narrow screen', async ({ page }) => {
  const id = await addSource(token, 'Only Source', 0);
  const items = await apiSearch(token);
  const series = items.find((i) => i.type === 'series' || i.type === 'anime');
  test.skip(!series, 'catalog served no series');

  const folder = folderNameFor(series!.title);
  const base = `${parentFor(series!.type)}/${folder}`;
  await seedLibrary({ [parentFor(series!.type)]: [folder], [base]: ['Season 01'] });
  await seedLibraryFiles({
    // A long episode list is the string that used to be truncated.
    [`${base}/Season 01`]: Array.from({ length: 12 }, (_, i) => `S.S01E${String(i + 1).padStart(2, '0')}.mkv`),
  });
  await refreshLibrary(token, id, 'Only Source');

  await page.setViewportSize({ width: 360, height: 780 });
  await login(page);
  await gotoDiscover(page);
  await page.getByTestId('catalog-card').filter({ hasText: series!.title }).first().click();

  const modal = page.locator('ion-modal').last();
  await expect(modal.getByTestId('season-group').first()).toBeVisible();

  // Nothing inside the sheet may stick out horizontally.
  const overflow = await modal.evaluate((root) => {
    const bounds = root.getBoundingClientRect();
    const bad: string[] = [];
    for (const el of Array.from(root.querySelectorAll('[data-testid="season-group"], .quality-row, .season-sub'))) {
      const r = el.getBoundingClientRect();
      if (r.width > 0 && (r.right > bounds.right + 1 || r.left < bounds.left - 1)) {
        bad.push(`${el.className} ${Math.round(r.left)}..${Math.round(r.right)}`);
      }
    }
    return bad;
  });
  expect(overflow, 'nothing may overflow the sheet at 360px').toEqual([]);
});
