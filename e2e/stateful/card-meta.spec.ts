/**
 * Release year and genre on title cards (spec 1032).
 *
 * The mock ZarFilm archive publishes both on every card — a year in a `year`
 * span and a genre in `genres_links` — which is exactly the shape the real site
 * uses, and exactly what the driver used to parse and then discard.
 */
import { expect, test } from '@playwright/test';
import { addSource, apiToken, clearSources, gotoDiscover, login, setSourceState } from './helpers';

let token = '';

test.beforeEach(async () => {
  token = await apiToken();
  await clearSources(token);
  await setSourceState('reset');
  await addSource(token, 'Mock ZarFilm', 0);
});

test('a title card shows the release year its source published', async ({ page }) => {
  await login(page);
  await gotoDiscover(page);
  const cards = page.getByTestId('catalog-card');
  await expect(cards.first()).toBeVisible({ timeout: 30_000 });

  // Every mock archive card carries a year, so the first one must show one.
  // Before spec 1032 this was blank for ZarFilm: its titles have no trailing
  // year, and the year field it did parse was dropped on the way out.
  const caption = await cards.first().innerText();
  expect(caption, `caption was: ${caption}`).toMatch(/\b20\d{2}\b/);
});

test('a title card shows a genre, in English', async ({ page }) => {
  await login(page);
  await gotoDiscover(page);
  const cards = page.getByTestId('catalog-card');
  await expect(cards.first()).toBeVisible({ timeout: 30_000 });

  // The mock publishes Comedy / Drama / Action. Whichever a card carries, it
  // must reach the caption as a readable English word.
  const captions = await cards.allInnerTexts();
  const withGenre = captions.filter((c) => /\b(Comedy|Drama|Action)\b/.test(c));
  expect(withGenre.length, `no genre on any card; first was: ${captions[0]}`).toBeGreaterThan(0);
});

test('the year is shown once, not twice', async ({ page }) => {
  await login(page);
  await gotoDiscover(page);
  const cards = page.getByTestId('catalog-card');
  await expect(cards.first()).toBeVisible({ timeout: 30_000 });

  // The heading has its trailing year stripped and the year is rendered as its
  // own field, so a card must never read "Mock Title 2014 · 2014".
  const caption = await cards.first().innerText();
  const years = caption.match(/\b20\d{2}\b/g) ?? [];
  expect(years.length, `caption was: ${caption}`).toBe(1);
});

// Spec 1032, FR-007. This is the assertion that was missing: it was left to a
// manual browser check that then could not be performed, so the one behaviour
// with no automated test was also the one nobody had looked at.
test('a card stops repeating the type once a type filter is applied', async ({ page }) => {
  await login(page);
  await gotoDiscover(page);
  const cards = page.getByTestId('catalog-card');
  await expect(cards.first()).toBeVisible({ timeout: 30_000 });

  // With no filter, the type IS part of the caption.
  const before = await cards.allInnerTexts();
  expect(before.some((c) => /\bMovie\b/.test(c)), `no Movie card to test with: ${before[0]}`).toBe(
    true,
  );

  await page.getByTestId('filter-open').click();
  await page.getByTestId('filter-type').click();
  // ion-select interface="alert" opens an Ionic alert of radio options.
  await page.locator('ion-alert button:has-text("Movie")').first().click();
  await page.locator('ion-alert button:has-text("OK")').click();
  await page.getByTestId('filter-apply').click();

  await expect(cards.first()).toBeVisible({ timeout: 30_000 });
  const after = await cards.allInnerTexts();
  // The filter chip already says Movie; repeating it on every card is a line of
  // meta saying nothing, and it costs the room the genre needs.
  expect(
    after.every((c) => !/\bMovie\b/.test(c)),
    `type still repeated after filtering: ${after.find((c) => /\bMovie\b/.test(c))}`,
  ).toBe(true);
  // ...and the genre is still there, which is what the freed room was for.
  expect(after.some((c) => /\b(Comedy|Drama|Action)\b/.test(c))).toBe(true);
});

// Spec 1035: the caption reads like the detail sheet — three lines, each
// answering a different question, rather than four facts run together on one.
test('a card caption is three lines: name, then rating/type/year, then genres', async ({ page }) => {
  await login(page);
  await gotoDiscover(page);
  const card = page.getByTestId('catalog-card').first();
  await expect(card).toBeVisible({ timeout: 30_000 });

  // Name on its own line, then the facts, then the genres.
  await expect(card.locator('h3')).toHaveCount(1);
  await expect(card.locator('p.facts')).toHaveCount(1);
  await expect(card.locator('p.genres')).toHaveCount(1);

  // The facts line reads rating, then type, then year.
  const facts = (await card.locator('p.facts').innerText()).replace(/\s+/g, ' ').trim();
  expect(facts, `facts line was: ${facts}`).toMatch(/^★\s*[\d.]+\s+\w+\s+\d{4}$/);

  // The genre line separates several genres the way the sheet does.
  const genres = await card.locator('p.genres').innerText();
  expect(genres.trim().length).toBeGreaterThan(0);
});

// FR-005: a title missing a fact must not make its card a different height,
// or the grid stops looking like a grid.
test('cards stay the same height when a title knows less about itself', async ({ page }) => {
  await login(page);
  await gotoDiscover(page);
  const cards = page.getByTestId('catalog-card');
  await expect(cards.first()).toBeVisible({ timeout: 30_000 });

  const heights = await cards.evaluateAll((els) =>
    els.slice(0, 8).map((el) => Math.round(el.getBoundingClientRect().height)),
  );
  const unique = [...new Set(heights)];
  expect(unique.length, `card heights differ: ${heights.join(', ')}`).toBe(1);
});
