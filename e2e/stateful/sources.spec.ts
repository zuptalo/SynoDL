/**
 * Multiple download sources, end to end (spec 0007, US1/US3).
 *
 * Everything here runs against fake source sites served by the mock, so the
 * suite needs no credentials and reaches no real site — which is the only way
 * this feature can be covered in CI at all.
 */
import { expect, test } from '@playwright/test';
import { addSource, apiToken, clearSources, gotoDiscover, login, setSourceState } from './helpers';

let token = '';

test.beforeEach(async () => {
  token = await apiToken();
  await clearSources(token);
  await setSourceState('reset');
});

test('with one source the picker stays hidden and Discover is unchanged', async ({ page }) => {
  await addSource(token, 'Only Source', 0);
  await login(page);
  await gotoDiscover(page);

  await expect(page.locator('.card').first()).toBeVisible();
  // FR-013: a selector with nothing to select is not shown.
  await expect(page.locator('.source-select')).toHaveCount(0);
  // FR-012a: nor is a source label, which would be noise on every card.
  await expect(page.locator('.src-mark')).toHaveCount(0);
});

test('two sources are combined by default, interleaved and labelled', async ({ page }) => {
  await addSource(token, 'Alpha Source', 0);
  await addSource(token, 'Beta Source', 1);
  await login(page);
  await gotoDiscover(page);

  const cards = page.locator('.card');
  await expect(cards.first()).toBeVisible();

  // FR-007: the selector appears, defaulting to all sources.
  await expect(page.locator('.source-select')).toBeVisible();
  await expect(page.locator('.source-select')).toContainText('All sources');

  // FR-009/SC-002: both sources are represented on the first screenful. The
  // label is a source mark now rather than text, so the name is read off its
  // title/alt — which is also what a screen reader gets.
  const marks = page.locator('.src-mark');
  const names = await marks.evaluateAll((els) => els.map((e) => e.getAttribute('title') ?? ''));
  expect(names).toContain('Alpha Source');
  expect(names).toContain('Beta Source');

  // FR-005a: the same title from both sources is TWO entries, not merged.
  const titles = await page.locator('.card .meta h3').allInnerTexts();
  const first = titles[0];
  expect(titles.filter((t) => t === first).length).toBeGreaterThan(1);
});

test('picking one source narrows the list and drops the labels', async ({ page }) => {
  await addSource(token, 'Alpha Source', 0);
  await addSource(token, 'Beta Source', 1);
  await login(page);
  await gotoDiscover(page);
  await expect(page.locator('.card').first()).toBeVisible();

  await page.locator('.source-select').click();
  await page.getByRole('radio', { name: 'Beta Source' }).click();
  await page.keyboard.press('Escape').catch(() => undefined);

  await expect(page.locator('.source-select')).toContainText('Beta Source', { timeout: 20_000 });
  // Only that source's results, so the label is redundant and suppressed.
  await expect(page.locator('.src-mark')).toHaveCount(0);
  await expect(page.locator('.card').first()).toBeVisible();

  // FR-008: the choice follows the user, so a reload keeps it.
  await page.reload();
  await expect(page.locator('.source-select')).toContainText('Beta Source', { timeout: 30_000 });
});

test('one source failing still shows the other, and says which is missing', async ({ page }) => {
  // The two sources sit on DIFFERENT fake sites, which is what makes it possible
  // to break exactly one of them and watch the other carry on. Backing both with
  // the same fake would break both and prove nothing.
  await addSource(token, 'Html Source', 0, 'zarfilm');
  await addSource(token, 'Json Source', 1, '30nama');
  await login(page);
  await gotoDiscover(page);
  await expect(page.locator('.card').first()).toBeVisible();

  await setSourceState('zar/logged-out');

  // A source is only condemned after several CONSECUTIVE failures — a lone blip
  // is treated as transient on purpose — so drive enough reloads to cross it.
  for (let i = 0; i < 6; i += 1) {
    await page.reload();
    await page.waitForTimeout(400);
  }

  // FR-012: the healthy source's results are still on screen...
  await expect(page.locator('.card').first()).toBeVisible({ timeout: 30_000 });
  // ...and the notice names the source that dropped out, so an operator knows
  // which one to go and fix.
  await expect(page.locator('.degraded')).toContainText('Html Source', { timeout: 30_000 });
  // The healthy source is still labelled on its results.
  await expect(page.locator('.src-mark').first()).toHaveAttribute('title', 'Json Source');

  await setSourceState('zar/logged-in');
});

test('an admin sees each source and its health, and never a stored secret', async ({ page }) => {
  await addSource(token, 'Alpha Source', 0);
  await addSource(token, 'Beta Source', 1);
  await login(page);

  await page.goto('/tabs/settings');
  await page.getByTestId('settings-source').click();

  await expect(page.getByText('Alpha Source')).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText('Beta Source')).toBeVisible();
  // Session material is write-only: the value pasted at creation must never be
  // rendered back anywhere on this screen.
  await expect(page.locator('body')).not.toContainText('e2e-not-a-real-cookie');
});
