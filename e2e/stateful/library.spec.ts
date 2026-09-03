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
  apiToken,
  clearSources,
  folderNameFor,
  gotoDiscover,
  login,
  refreshLibrary,
  seedLibrary,
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
  await seedLibrary({ [parentFor(owned.type)]: [folderNameFor(owned.title)] });
  await refreshLibrary(token, id, 'Only Source');

  // FR-001: the API now reports exactly that one title as present.
  const after = await apiSearch(token);
  expect(after.find((i) => i.title === owned.title)?.inLibrary).toBe(true);
  expect(after.filter((i) => i.inLibrary).length).toBe(1);

  await login(page);
  await gotoDiscover(page);

  // FR-011/FR-012: one card carries the marker, and it is announced rather than
  // conveyed by colour alone.
  const marker = page.locator('.badge-have');
  await expect(marker.first()).toBeVisible();
  await expect(marker).toHaveCount(1);
  await expect(marker.first()).toHaveAttribute('aria-label', /already in your library/i);
});

test('nothing is marked when the NAS holds none of the catalog', async ({ page }) => {
  await addSource(token, 'Only Source', 0);
  await login(page);
  await gotoDiscover(page);

  await expect(page.locator('.card').first()).toBeVisible();
  await expect(page.locator('.badge-have')).toHaveCount(0);
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
  expect(after.find((i) => i.title === subject.title)?.inLibrary ?? false).toBe(false);

  await login(page);
  await gotoDiscover(page);
  await expect(page.locator('.card').first()).toBeVisible();
  await expect(page.locator('.badge-have')).toHaveCount(0);
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
  await expect(page.locator('.badge-have')).toHaveCount(0);
  // The failed scan is invisible: no empty/error state took over the grid.
  await expect(page.locator('.state')).toHaveCount(0);
});
