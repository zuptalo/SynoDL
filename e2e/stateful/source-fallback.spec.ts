/**
 * Opening on a source that has since gone down (spec 2010).
 *
 * The saved selection is restored on open, so a source that stopped answering
 * puts Discover straight into the full-screen "needs refreshing" state — which
 * replaces the toolbar, and with it the source picker. There was then no way to
 * reach the healthy source at all.
 */
import { expect, test } from '@playwright/test';
import {
  addSource,
  apiToken,
  clearSources,
  getSelectedSource,
  gotoDiscover,
  login,
  setSelectedSource,
  setSourceState,
} from './helpers';

let token = '';

test.beforeEach(async () => {
  token = await apiToken();
  await clearSources(token);
  await setSourceState('reset');
});

test('opening on a source that is down falls back to the healthy ones', async ({ page }) => {
  const zarID = await addSource(token, 'Flaky Source', 0, 'zarfilm');
  await addSource(token, 'Healthy Source', 1, '30nama');
  // The user was last looking at the one that has since stopped answering.
  await setSelectedSource(token, String(zarID));
  await setSourceState('zar/logged-out');

  await login(page);
  await gotoDiscover(page);

  // Not stranded on the dead end.
  await expect(page.getByText('The download source needs refreshing.')).toHaveCount(0);
  // The healthy source's catalog is on screen.
  await expect(page.getByTestId('catalog-card').first()).toBeVisible({ timeout: 30_000 });
  // And the view has moved to everything, so the picker is reachable again.
  await expect(page.locator('.source-select')).toContainText('All sources');
  // The reason is stated rather than left to be guessed at.
  await expect(page.locator('.degraded')).toContainText('Flaky Source');
});

// The stored preference is not overwritten: when the source comes back the user
// is where they left off, rather than having been quietly moved.
test('falling back does not rewrite what the user chose', async ({ page }) => {
  const zarID = await addSource(token, 'Flaky Source', 0, 'zarfilm');
  await addSource(token, 'Healthy Source', 1, '30nama');
  await setSelectedSource(token, String(zarID));
  await setSourceState('zar/logged-out');

  await login(page);
  await gotoDiscover(page);
  await expect(page.getByTestId('catalog-card').first()).toBeVisible({ timeout: 30_000 });

  // Read through the API with the session, not page.request, which carries none.
  expect(await getSelectedSource(token)).toBe(String(zarID));
});

// A single source that is down has nothing to fall back to, so the honest screen
// stays — falling back must not paper over "everything is broken".
test('with nothing healthy to show, the honest screen stays', async ({ page }) => {
  await addSource(token, 'Flaky Source', 0, 'zarfilm');
  await setSourceState('zar/logged-out');

  await login(page);
  await gotoDiscover(page);

  await expect(page.getByText('The download source needs refreshing.')).toBeVisible({ timeout: 30_000 });
});
