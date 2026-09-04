/**
 * Choosing a quality and sending it (spec 1027).
 *
 * This flow had no coverage at all: the whole suite could pass while the sheet
 * opened with a pre-selected option the user never chose — one that, on a
 * part-owned series, sat inside a COLLAPSED season and armed the send button
 * with a season they already had.
 */
import { expect, test, type Page } from '@playwright/test';
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
  seedLibraryFiles,
  setSourceState,
} from './helpers';

let token = '';
const parentFor = (type: string) => (type === 'series' || type === 'anime' ? '/tv-show' : '/movie');

/** Ionic reflects `disabled` as a property, so read the property. */
const sendDisabled = (page: Page) =>
  page.locator('ion-modal .send-btn').evaluate((el) => (el as unknown as { disabled: boolean }).disabled);

const chosen = (page: Page) =>
  page.locator('ion-modal ion-radio-group').evaluate((el) => (el as unknown as { value: string }).value);

/** Seed a series with seasons 1-2 on the NAS and 3 missing, and open it. */
async function openPartlyOwnedSeries(page: Page): Promise<string> {
  const id = await addSource(token, 'Only Source', 0);
  const items = await apiSearch(token);
  const series = items.find((i) => i.type === 'series' || i.type === 'anime');
  test.skip(!series, 'catalog served no series');

  const folder = folderNameFor(series!.title);
  const base = `${parentFor(series!.type)}/${folder}`;
  await seedLibrary({ [parentFor(series!.type)]: [folder], [base]: ['Season 01', 'Season 02'] });
  await seedLibraryFiles({
    [`${base}/Season 01`]: ['Mock.S01E01.1080p.WEB-DL.x264.Alpha.MockSite.mkv'],
    [`${base}/Season 02`]: ['Mock.S02E01.1080p.WEB-DL.Dubbed.MockSite.mkv'],
  });
  await refreshLibrary(token, id, 'Only Source');

  await login(page);
  await gotoDiscover(page);
  await page.getByTestId('catalog-card').filter({ hasText: series!.title }).first().click();
  await expect(page.getByTestId('season-group').first()).toBeVisible();
  return series!.title;
}

test.beforeEach(async () => {
  token = await apiToken();
  await clearSources(token);
  await setSourceState('reset');
});

test('a sheet opens with nothing chosen, and cannot be sent', async ({ page }) => {
  await openPartlyOwnedSeries(page);

  // A pre-selected option reads as a decision the user made. It also used to be
  // INVISIBLE here — season 1's option, inside a collapsed season — so the send
  // button was armed with a season they already had.
  expect(await chosen(page)).toBe('');
  expect(await sendDisabled(page)).toBe(true);
  await expect(page.locator('ion-modal ion-radio[aria-checked="true"]')).toHaveCount(0);
});

test('choosing a quality arms the send button and reveals the episodes', async ({ page }) => {
  await openPartlyOwnedSeries(page);

  // Only the open season's options are reachable, which is the point.
  const options = page.locator('ion-modal ion-radio:visible');
  await expect(options).toHaveCount(2);
  await options.first().click();

  expect(await chosen(page)).not.toBe('');
  expect(await sendDisabled(page)).toBe(false);
  // A series lets the user trim the episodes before sending.
  await expect(page.locator('ion-modal .ep-list ion-checkbox').first()).toBeVisible();
  await expect(page.locator('ion-modal .send-btn')).toContainText('Send 4 to NAS');
});

test('opening another season drops a choice you can no longer see', async ({ page }) => {
  await openPartlyOwnedSeries(page);

  await page.locator('ion-modal ion-radio:visible').first().click();
  expect(await chosen(page)).not.toBe('');

  // Season 1 is a different group; the previous pick is now hidden.
  await page.getByTestId('season-group').first().click();
  await expect.poll(() => chosen(page)).toBe('');
  expect(await sendDisabled(page)).toBe(true);
});

test('a season you do not have is sent without being second-guessed', async ({ page }) => {
  await openPartlyOwnedSeries(page);

  // The open season is the first one missing, so this is a genuinely new download.
  await page.locator('ion-modal ion-radio:visible').first().click();
  await page.locator('ion-modal .send-btn').click();

  // No "you already have this" prompt: the title counts as owned because of
  // seasons 1 and 2, but this is not one of them.
  await expect(page.locator('ion-alert')).toHaveCount(0);

  // The button becomes the live status control for what was just created — which
  // is why there is no toast as well.
  await expect(page.locator('ion-modal .send-btn')).toContainText('View in Tasks', { timeout: 15_000 });
  await expect(page.locator('ion-toast:visible')).toHaveCount(0);
});

test('a season you already have still asks first', async ({ page }) => {
  await openPartlyOwnedSeries(page);

  // Season 1 IS on the NAS. Wait for the accordion to settle before reaching for
  // an option: mid-animation, ":visible" also matches rows in the season that is
  // collapsing, and those never come to rest.
  await page.getByTestId('season-group').first().click();
  await expect
    .poll(() => page.locator('ion-modal ion-accordion-group').evaluate((el) => (el as unknown as { value: string }).value))
    .toBe('1');
  // Scoped by position, not by [value]: Ionic takes `value` as a PROPERTY, so an
  // attribute selector matches nothing. Season 1 is the first accordion.
  await page.locator('ion-modal ion-accordion').first().locator('ion-radio').first().click();
  await page.locator('ion-modal .send-btn').click();

  const alert = page.locator('ion-alert');
  await expect(alert).toBeVisible();
  await expect(alert).toContainText('already have season 1');
  // Cancelling sends nothing. Ionic wraps an alert button's label in an inner
  // span, so match the button by its own class rather than by role.
  await alert.locator('.alert-button').filter({ hasText: 'Cancel' }).click();
  await expect(page.locator('ion-modal .send-btn')).not.toContainText('View in Tasks');
});

test('a movie has no episode picker and sends straight away', async ({ page }) => {
  await addSource(token, 'Only Source', 0);
  const items = await apiSearch(token);
  const movie = items.find((i) => i.type === 'movie');
  test.skip(!movie, 'catalog served no movie');

  await login(page);
  await gotoDiscover(page);
  await page.getByTestId('catalog-card').filter({ hasText: movie!.title }).first().click();
  await expect(page.locator('ion-modal ion-radio').first()).toBeVisible();

  expect(await chosen(page)).toBe('');
  expect(await sendDisabled(page)).toBe(true);

  await page.locator('ion-modal ion-radio:visible').first().click();
  expect(await sendDisabled(page)).toBe(false);
  // Nothing to trim on a movie — the next step is the send button itself.
  await expect(page.locator('ion-modal .ep-list')).toHaveCount(0);
  await expect(page.locator('ion-modal .send-btn')).toContainText('Send to NAS');
});
