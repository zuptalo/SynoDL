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

/**
 * Start a search WITHOUT pulling.
 *
 * Since spec 2028's redesign a pull carries its own cancel, inside the ring it
 * draws, and the floating block is suppressed while one is running — one spinner
 * on screen, not two. So the block's own tests have to start a search the way
 * the block is for: a typed query, a sort, a filter. Typing changes nothing in
 * the layout below the header, so anything that moves afterwards moved because
 * of the block.
 */
async function searchWithoutPulling(page: Page): Promise<void> {
  // The sort DIRECTION toggle: one click, no popover, and — unlike typing a
  // query, which reveals the search hint — nothing above the grid changes size.
  // That matters here, because "nothing moves" is the assertion.
  await page.locator('.order-toggle').click();
}

/**
 * Pull the list down for real, with a pointer.
 *
 * Ionic's refresher is a gesture, and dispatching `ionRefresh` at it — which the
 * older tests here do, legitimately, to ask "did it search?" — skips everything
 * the gesture drives: the class it sets, the transform it applies, and therefore
 * the whole animation. Nothing about the indicator can be tested that way.
 *
 * Returns, sampled at every step: the scroller's vertical offset, and the drawn
 * height of the droplet. SC-003 is about the second one.
 */
async function dragDown(
  page: Page,
  steps = 16,
  px = 9,
): Promise<{ offsets: number[]; drop: number[] }> {
  const box = await page.locator('ion-content').first().boundingBox();
  // Down the LEFT GUTTER of the grid, not through a card. A pointer drag that
  // starts and ends on a card still delivers a click to it, so dragging through
  // the middle opened the title modal on release — the pull worked, and the
  // assertions after it were then talking to a dialog.
  const x = Math.round((box?.x ?? 0) + 8);
  const y = Math.round((box?.y ?? 0) + 120);
  const offsets: number[] = [];
  const drop: number[] = [];
  await page.mouse.move(x, y);
  await page.mouse.down();
  for (let i = 1; i <= steps; i += 1) {
    await page.mouse.move(x, y + i * px);
    const s = await page.evaluate(() => {
      const el = document.querySelector('ion-content')?.shadowRoot?.querySelector('.inner-scroll');
      const t = el ? getComputedStyle(el).transform : 'none';
      const path = document.querySelector('.pr-stage path') as SVGGraphicsElement | null;
      return {
        offset: t && t !== 'none' ? new DOMMatrixReadOnly(t).m42 : 0,
        // The DRAWN shape, in its own user units — which is the only thing that
        // can say whether the animation restarted.
        drop: path ? Math.round(path.getBBox().height) : 0,
      };
    });
    offsets.push(s.offset);
    drop.push(s.drop);
    await page.waitForTimeout(16);
  }
  return { offsets, drop };
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
  await searchWithoutPulling(page);

  const pill = page.getByTestId('search-cancel');
  await expect(pill).toBeVisible({ timeout: 15_000 });

  // SC-005. Flipping the sort direction changes nothing below the header, so if
  // the grid has not moved, the pill costs no layout — which is the whole reason it is laid out
  // over the content rather than in it.
  const gridDuring = await page.getByTestId('catalog-card').first().boundingBox();
  expect(gridDuring?.y, 'the grid moved when the cancel pill appeared').toBe(gridBefore?.y);

  // The spinner and the cancel are ONE block: the cancel sits directly beneath
  // the spinner, and they keep that relationship however the list moves.
  // Asserted as a relationship rather than two absolute positions — absolute
  // positions would pass with them anywhere on screen.
  const block = page.locator('.search-block');
  await expect(block).toHaveCSS('position', 'fixed');
  const spinner = await block.locator('.block-spinner').boundingBox();
  const button = await pill.boundingBox();
  expect(button?.y ?? 0, 'the cancel is not below the spinner').toBeGreaterThan(
    (spinner?.y ?? 0) + (spinner?.height ?? 0) - 1,
  );
  // Centred on each other, so the pair reads as one thing rather than two.
  const spinnerMid = (spinner?.x ?? 0) + (spinner?.width ?? 0) / 2;
  const buttonMid = (button?.x ?? 0) + (button?.width ?? 0) / 2;
  expect(Math.abs(spinnerMid - buttonMid)).toBeLessThan(2);

  // And it sits clear of the header, measured from the real one.
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
  await searchWithoutPulling(page);

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

/**
 * The pull indicator (spec 2028, redesigned).
 *
 * Driven by a real pointer drag, because every part of this feature is the
 * gesture's: the shape is redrawn from the transform Ionic applies, and the ring
 * appears when Ionic sets its own class. A synthetic `ionRefresh` produces
 * neither, so it would assert nothing about what a reader sees.
 */
test('pulling draws one indicator, growing downward, with the cancel inside it', async ({
  page,
}) => {
  await login(page);
  await openDiscover(page);
  await expect(page.locator('.card').first()).toBeVisible({ timeout: 30_000 });
  await page.waitForTimeout(3000);

  await setSlow(6000);
  const { offsets, drop } = await dragDown(page);

  // SC-003: the shape never restarts mid-pull.
  //
  // Measured on the DRAWN SHAPE, not on the scroll offset — the offset can sit
  // still while the shape is redrawn from something else, which is exactly how
  // the original bug behaved.
  //
  // Read what this does NOT cover. The reported restart came from measuring the
  // pull from two sources, Ionic's transform and the browser's rubber-band
  // overscroll, and desktop Chrome has no elastic overscroll on an inner
  // scroller — restoring the two-source measurement leaves this test passing.
  // Checked, rather than assumed. That invariant is held in the unit tests for
  // `pull-distance`, which is why it is a function taking only a transform. What
  // this covers is the rest of it: that a real gesture draws the shape at all,
  // and that it does not collapse for any reason reachable from here.
  //
  // The floor is 20, not "never decreases", because the shape legitimately gets
  // SHORTER at the end: the tail pinches off into the ring, settling at the
  // circle's 30. A restart lands at the dot's 6.
  expect(drop.some((h) => h > 20), `the droplet never drew: ${drop.join(', ')}`).toBe(true);
  const grown = drop.findIndex((h) => h > 20);
  expect(
    drop.slice(grown).filter((h) => h < 20),
    `the droplet collapsed and restarted mid-pull: ${drop.join(', ')}`,
  ).toEqual([]);
  expect(offsets.at(-1) ?? 0, 'the pull never opened').toBeGreaterThan(0);

  await page.mouse.up();

  // FR-002. One spinner. The floating block is the treatment for a search
  // started any OTHER way; while a pull is running it must stay away, which is
  // what the two-spinners-a-few-pixels-apart bug was.
  const ring = page.getByTestId('pull-refresh');
  await expect(ring).toBeVisible({ timeout: 15_000 });
  await expect(page.getByTestId('search-cancel')).toHaveCount(0);

  // SC-001. The label sits above the ring, on its centre line — asserted as a
  // relationship, since absolute positions would pass with the pair anywhere.
  // Measured against the RING, not the button: the button is deliberately padded
  // out past the ring to a finger-sized target, so its box would report the
  // label overlapping it while the drawn shapes are clear of each other.
  const label = await page.locator('.pr-label').boundingBox();
  const circle = await page.locator('[data-testid="pull-refresh"] circle').boundingBox();
  const cross = await page.locator('[data-testid="pull-refresh"] path').boundingBox();
  expect(
    (label?.y ?? 0) + (label?.height ?? 0),
    'the label is not above the ring',
  ).toBeLessThanOrEqual(circle?.y ?? 0);
  const mid = (b: typeof circle) => ({
    x: (b?.x ?? 0) + (b?.width ?? 0) / 2,
    y: (b?.y ?? 0) + (b?.height ?? 0) / 2,
  });
  expect(
    Math.abs(mid(label).x - mid(circle).x),
    'the label is not centred on the ring',
  ).toBeLessThan(2);
  // The ✕ is IN the ring, not beside it — and inside its bounds, not merely
  // sharing a centre with something twice its size.
  expect(Math.abs(mid(cross).x - mid(circle).x), 'the cross is not centred').toBeLessThan(1.5);
  expect(Math.abs(mid(cross).y - mid(circle).y), 'the cross is not centred').toBeLessThan(1.5);
  expect((cross?.width ?? 0) < (circle?.width ?? 0), 'the cross is not inside the ring').toBe(true);

  // FR-009. Drawn in the gap the refresher holds open, never over a row. The
  // first card has been pushed below the ring rather than sitting under it.
  const card = await page.getByTestId('catalog-card').first().boundingBox();
  expect(card?.y ?? 0, 'the indicator is covering the list').toBeGreaterThanOrEqual(
    (circle?.y ?? 0) + (circle?.height ?? 0),
  );

  // FR-007. The ✕ is reachable and retracts the refresher — it did nothing at
  // all while Ionic's `z-index: -1` left the list painted over it, and a test
  // that only checked the button was visible would have passed throughout.
  await ring.click();
  await expect(ring).toHaveCount(0, { timeout: 10_000 });
  await expect
    .poll(async () =>
      page.evaluate(
        () => document.querySelector('ion-refresher')?.classList.contains('refresher-refreshing'),
      ),
    )
    .toBe(false);

  await setSlow(0);
});
