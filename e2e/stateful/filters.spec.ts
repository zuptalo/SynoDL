/**
 * Filtering and sorting across sources (spec 1024).
 *
 * The two fake sources are deliberately shaped like the real pair: one publishes
 * opaque numeric genre codes, the other its own words, and only an English slug
 * says the two mean the same genre. So these tests exercise the translation, not
 * a pair of look-alikes that would agree however it was implemented.
 */
import { expect, test } from '@playwright/test';
import {
  addSource,
  apiParameters,
  apiSearch,
  apiToken,
  clearSources,
  gotoDiscover,
  login,
  setSourceState,
  slugs,
} from './helpers';

let token = '';

test.beforeEach(async () => {
  token = await apiToken();
  await clearSources(token);
  await setSourceState('reset');
});

test('the HTML-shaped source declares what it can filter and sort by', async () => {
  await addSource(token, 'Only Source', 0);
  const p = await apiParameters(token);

  expect(slugs(p.genres)).toEqual(expect.arrayContaining(['comedy', 'drama', 'action']));
  expect(slugs(p.sorts)).toEqual(expect.arrayContaining(['imdb', 'year', 'date', 'favorite']));
  expect(p.scores?.length ?? 0).toBeGreaterThan(0);
});

test('a genre actually narrows the results', async () => {
  await addSource(token, 'Only Source', 0);
  const p = await apiParameters(token);
  const comedy = p.genres.find((g) => g.slug === 'comedy');
  expect(comedy, 'the source should offer comedy').toBeTruthy();

  const all = await apiSearch(token, { page: 1 });
  const narrowed = await apiSearch(token, { page: 1, filters: { genre: [comedy!.value] } });

  // Something was filtered out, and everything left is the genre asked for.
  expect(narrowed.length).toBeGreaterThan(0);
  expect(narrowed.length).toBeLessThan(all.length);
  for (const item of narrowed) {
    expect(item.genres ?? []).toContain('Comedy');
  }
});

// FR-002: the sort used to be written with a parameter the site does not read,
// so it silently did nothing. Ordering must visibly change.
test('sorting by rating actually reorders the results', async () => {
  await addSource(token, 'Only Source', 0);

  const byRating = await apiSearch(token, { page: 1, sort: 'imdb' });
  const scores = byRating.map((i) => i.imdbScore ?? 0);
  const descending = [...scores].sort((a, b) => b - a);

  expect(scores.length).toBeGreaterThan(1);
  expect(scores).toEqual(descending);
});

test('combined browsing offers what both sources can do, and nothing more', async () => {
  await addSource(token, 'Words Source', 0, 'zarfilm');
  await addSource(token, 'Codes Source', 1, '30nama');

  const combined = await apiParameters(token);
  const shared = slugs(combined.genres);

  // Both carry these.
  expect(shared).toEqual(expect.arrayContaining(['comedy', 'drama']));
  // Each source has one the other lacks; neither may be offered for a view that
  // mixes them, or it would filter half the grid and quietly not the rest.
  expect(shared).not.toContain('action');
  expect(shared).not.toContain('period-drama');
  // Same rule for orderings: "recently updated" is one source's alone.
  expect(slugs(combined.sorts)).not.toContain('modified');
  expect(slugs(combined.sorts)).toEqual(expect.arrayContaining(['imdb', 'favorite']));
});

test('a single source gets its full set back', async () => {
  const zarID = await addSource(token, 'Words Source', 0, 'zarfilm');
  await addSource(token, 'Codes Source', 1, '30nama');

  const solo = await apiParameters(token, String(zarID));

  expect(slugs(solo.genres)).toContain('action');
  expect(slugs(solo.sorts)).toContain('modified');
});

// FR-006, the heart of the spec: one chosen value, two vocabularies, both
// sources answering.
test('one chosen genre is understood by both sources at once', async () => {
  await addSource(token, 'Words Source', 0, 'zarfilm');
  await addSource(token, 'Codes Source', 1, '30nama');

  const combined = await apiParameters(token);
  const comedy = combined.genres.find((g) => g.slug === 'comedy');
  expect(comedy, 'comedy should be shared').toBeTruthy();

  const items = await apiSearch(token, { page: 1, filters: { genre: [comedy!.value] } });
  expect(items.length).toBeGreaterThan(0);

  // Both sources contributed — so the value was rewritten for at least one of
  // them, since a single literal cannot be valid on both.
  const contributors = new Set(items.map((i) => i.sourceName));
  expect(contributors).toContain('Words Source');
  expect(contributors).toContain('Codes Source');
  // Compared case-insensitively on purpose: the two drivers report a title's
  // genres differently (one passes the site's label through, the other the
  // slug). That is a pre-existing cosmetic difference, and pinning it here would
  // make this test fail for a reason that has nothing to do with filtering.
  for (const item of items) {
    expect((item.genres ?? []).map((g) => g.toLowerCase()), item.sourceName).toContain('comedy');
  }
});

test('the sort control offers the live orderings', async ({ page }) => {
  await addSource(token, 'Only Source', 0);
  await login(page);
  await gotoDiscover(page);

  await page.locator('.sort-select').click();
  // "Recently updated" is this source's own ordering — it can only be on screen
  // if the control is built from what the source declared, not a built-in list.
  await expect(page.getByRole('radio', { name: 'Recently updated' })).toBeVisible();
});
