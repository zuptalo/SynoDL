/**
 * Discover opens where you left it, and a search can be called off (spec 1041).
 *
 * Both halves are about the same thing — the reader deciding when the source is
 * asked — so both are measured the same way: by COUNTING REQUESTS, not by
 * watching the screen. A grid that looks right says nothing about whether it was
 * fetched, and "did it search?" is the whole question here.
 */
import { expect, test, type Page } from '@playwright/test';
import { addSource, apiToken, clearSources, login, setSourceState } from './helpers';

let token = '';

test.beforeEach(async () => {
  token = await apiToken();
  await clearSources(token);
  await setSourceState('reset');
  await addSource(token, 'Only Source', 0);
});

/** Count catalog searches — the asking this feature exists to put in the reader's hands. */
function countSearches(page: Page): () => number {
  let n = 0;
  page.on('request', (r) => {
    if (new URL(r.url()).pathname === '/v1/source/search') n += 1;
  });
  return () => n;
}

/**
 * Make the fake source answer slowly.
 *
 * A real source sometimes does, and it is the only way to still have a search
 * running when a test wants to call it off — the whole feature is about the
 * window between asking and answering.
 */
async function setSlow(ms: number): Promise<void> {
  await setSourceState(`zar/slow?ms=${ms}`);
}

async function openDiscover(page: Page): Promise<void> {
  await page.goto('/tabs/browser');
  await expect(page.locator('.card, .state').first()).toBeVisible({ timeout: 30_000 });
}

test('the first ever visit searches, because there is nothing to come back to', async ({ page }) => {
  await login(page);
  const searches = countSearches(page);
  await openDiscover(page);
  await expect(page.locator('.card').first()).toBeVisible({ timeout: 30_000 });
  expect(searches(), 'a first run has no session to restore, so it must fetch').toBeGreaterThan(0);
});

test('opening the app again shows the last session without asking the source', async ({ page }) => {
  // Session one: browse, which is what gets remembered.
  await login(page);
  await openDiscover(page);
  await expect(page.locator('.card').first()).toBeVisible({ timeout: 30_000 });
  // The TITLE, not the card's whole text: a card also carries a source mark and
  // an ownership ribbon, which are rendered from live state and are not what
  // "the last session's results" means.
  const firstTitle = await page.getByTestId('catalog-card').first().locator('h3').innerText();

  // Let the first session finish before counting. Its fill-the-viewport loop
  // keeps fetching pages after the first card renders, and those land AFTER a
  // counter attached here — which is session one's tail being blamed on session
  // two. This is the mistake this test made first time round.
  await page.waitForTimeout(4000);

  // A reload is a fresh app open: the page is new, and only what was written to
  // the device survives.
  const searches = countSearches(page);
  await page.reload();
  await openDiscover(page);

  await expect(page.locator('.card').first()).toBeVisible({ timeout: 30_000 });
  expect(await page.getByTestId('catalog-card').first().locator('h3').innerText()).toBe(firstTitle);
  expect(searches(), 'opening the app searched instead of showing the last session').toBe(0);
});

test('pulling to refresh still asks, so fresh results are one gesture away', async ({ page }) => {
  await login(page);
  await openDiscover(page);
  await expect(page.locator('.card').first()).toBeVisible({ timeout: 30_000 });

  await page.reload();
  await openDiscover(page);
  const searches = countSearches(page);

  // The refresher is a gesture; calling what it calls is the honest equivalent
  // in a test, and it is the same code path.
  await page.evaluate(() => {
    document.querySelector('ion-refresher')?.dispatchEvent(new CustomEvent('ionRefresh', {
      detail: { complete: () => undefined },
      bubbles: true,
    }));
  });
  await expect.poll(() => searches(), { timeout: 20_000 }).toBeGreaterThan(0);
});

/** Start a search the way the refresher does — it changes nothing in the header,
 *  so anything that moves afterwards moved because of the pill. */
async function pullToRefresh(page: Page): Promise<void> {
  await page.evaluate(() => {
    document.querySelector('ion-refresher')?.dispatchEvent(
      new CustomEvent('ionRefresh', { detail: { complete: () => undefined }, bubbles: true }),
    );
  });
}

test('a search in progress offers to be called off, and nothing moves when it appears', async ({
  page,
}) => {
  await login(page);
  await openDiscover(page);
  await expect(page.locator('.card').first()).toBeVisible({ timeout: 30_000 });
  await page.waitForTimeout(3000); // let the first fill settle

  // Nothing running, nothing offering to cancel (FR-011).
  await expect(page.getByTestId('search-cancel')).toHaveCount(0);
  const gridBefore = await page.getByTestId('catalog-card').first().boundingBox();

  await setSlow(5000);
  await pullToRefresh(page);

  const pill = page.getByTestId('search-cancel');
  await expect(pill).toBeVisible({ timeout: 15_000 });

  // SC-005. The refresher changes nothing in the header, so if the grid has not
  // moved, the pill costs no layout — which is the whole reason it is laid out
  // over the content rather than in it.
  const gridDuring = await page.getByTestId('catalog-card').first().boundingBox();
  expect(gridDuring?.y, 'the grid moved when the cancel pill appeared').toBe(gridBefore?.y);
  await expect(pill).toHaveCSS('position', 'fixed');

  // And it sits BELOW the refresher's spinner, measured from the real header.
  //
  // Asserting merely "not overlapping the header" is not enough, and that is not
  // a guess: the pill was previously placed at a hard-coded 108px offset, which
  // clears this viewport's short header and so passed such a test — while
  // landing squarely on the search box on a phone. This checks the clearance the
  // design actually asks for, which is what the fallback could never satisfy.
  const header = await page.locator('ion-header').first().boundingBox();
  const headerBottom = (header?.y ?? 0) + (header?.height ?? 0);
  const box = await pill.boundingBox();
  expect(
    box?.y ?? 0,
    'the cancel pill is not clear of the header and the refresher spinner',
  ).toBeGreaterThanOrEqual(headerBottom + 80);

  await pill.click();
  await expect(pill).toHaveCount(0, { timeout: 10_000 });
  // FR-010: usable again immediately, rather than waiting out the request.
  await expect(page.getByTestId('filter-open')).toBeEnabled();

  await setSlow(0);
});

test('cancelling puts the view back, so the controls never describe what is not shown', async ({
  page,
}) => {
  // FR-009. Leaving the typed query above the old results would make the screen
  // claim a view it is not showing — and nothing about that looks wrong.
  await login(page);
  await openDiscover(page);
  await expect(page.locator('.card').first()).toBeVisible({ timeout: 30_000 });
  await page.waitForTimeout(3000);

  await setSlow(5000);
  await page.getByTestId('discover-search').locator('input').fill('dune');

  const pill = page.getByTestId('search-cancel');
  await expect(pill).toBeVisible({ timeout: 15_000 });
  await pill.click();
  await expect(pill).toHaveCount(0, { timeout: 10_000 });

  await expect(page.getByTestId('discover-search').locator('input')).toHaveValue('');
  await setSlow(0);
});

test('calling a search off stops it reaching the source', async ({ page }) => {
  await login(page);
  await openDiscover(page);
  await expect(page.locator('.card').first()).toBeVisible({ timeout: 30_000 });
  await page.waitForTimeout(3000);

  await setSlow(5000);
  const searches = countSearches(page);
  await pullToRefresh(page);

  const pill = page.getByTestId('search-cancel');
  await expect(pill).toBeVisible({ timeout: 15_000 });
  await pill.click();

  const afterCancel = searches();
  // Long enough that the abandoned search's fill-the-viewport loop would have
  // fired several times over had it not been called off.
  await page.waitForTimeout(9000);
  expect(searches(), 'pages kept being fetched after the search was called off').toBe(afterCancel);

  await setSlow(0);
});
