/**
 * A title sheet describes the title, whatever the source is (spec 1023).
 *
 * The sheet's header metadata comes from the catalog entry, and a source whose
 * listing pages carry no synopsis and no IMDb link (the HTML-shaped one) leaves
 * it bare. The title response carries both instead, read from the page the
 * source is already fetching for its download options.
 */
import { expect, test } from '@playwright/test';
import { addSource, apiSearch, apiToken, apiTitle, clearSources, gotoDiscover, login, setSourceState } from './helpers';

let token = '';

test.beforeEach(async () => {
  token = await apiToken();
  await clearSources(token);
  await setSourceState('reset');
});

test('a title carries its synopsis and IMDb id, for a movie and for a series', async () => {
  await addSource(token, 'Only Source', 0);
  const items = await apiSearch(token);
  const movie = items.find((i) => i.type === 'movie');
  const series = items.find((i) => i.type === 'series' || i.type === 'anime');
  // Skip rather than substitute: this asserts the two page shapes are handled
  // alike, so covering one twice would prove nothing about the other.
  test.skip(!movie || !series, 'catalog served only one kind of title');

  for (const t of [movie!, series!]) {
    const detail = await apiTitle(token, t.id);
    expect(detail.plot, `${t.type} synopsis`).toBeTruthy();
    expect(detail.imdbId, `${t.type} imdb id`).toMatch(/^tt\d{7,10}$/);
    // Metadata is an addition: the download options are still the point.
    expect(detail.qualities?.length ?? 0).toBeGreaterThan(0);
  }
});

test('the sheet shows the synopsis and links out to IMDb', async ({ page }) => {
  await addSource(token, 'Only Source', 0);
  await login(page);
  await gotoDiscover(page);

  await page.getByTestId('catalog-card').first().click();

  const plot = page.locator('.plot');
  await expect(plot).toBeVisible();
  await expect(plot).not.toBeEmpty();
  // The synopsis arrives in whatever language the source publishes, so the
  // paragraph asks the browser to pick its own direction rather than assuming.
  await expect(plot).toHaveAttribute('dir', 'auto');

  await expect(page.locator('a.imdb-link')).toHaveAttribute(
    'href',
    /^https:\/\/www\.imdb\.com\/title\/tt\d{7,10}\/?$/,
  );
});
