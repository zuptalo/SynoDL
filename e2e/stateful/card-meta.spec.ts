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
