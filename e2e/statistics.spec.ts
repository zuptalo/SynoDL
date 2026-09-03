/**
 * Spec 0006 — download statistics.
 *
 * NOTE ON SCOPE: statistics are a STATEFUL-mode feature (they need SynoDL user
 * accounts + the download_history table). The e2e harness boots synodl in the
 * legacy STATELESS mode (the client authenticates straight to the mock DSM), so
 * the statistics surface is intentionally inert here. This spec therefore guards
 * the gating — the stateful-only UI must NOT leak into stateless mode — which is
 * a meaningful regression check runnable in the current harness. The stateful
 * behaviour (counts, averages, the history graph, role gating) is covered by the
 * server unit/integration tests (internal/api, internal/push, internal/store)
 * and the client unit tests (stats-buckets). A full stateful stats e2e awaits a
 * stateful e2e harness (tracked separately).
 */
import { expect, test } from '@playwright/test';
import { login, openNewTask, resetMock } from './helpers';

test.beforeEach(async () => {
  await resetMock();
});

test('the Statistics section is hidden in stateless mode', async ({ page }) => {
  await login(page);
  await page.getByTestId('tab-settings').click();

  // Settings renders, but the stateful-only Statistics row is absent.
  await expect(page.getByTestId('settings-version')).toBeVisible();
  await expect(page.getByTestId('settings-statistics')).toHaveCount(0);
});

test('the new-task category picker is hidden in stateless mode', async ({ page }) => {
  await login(page); // lands on Tasks
  await openNewTask(page);

  // The add-task modal opens (URL box present) but carries no category picker,
  // since categories only feed the stateful statistics.
  await expect(page.getByTestId('newtask-urls')).toBeVisible();
  await expect(page.getByTestId('newtask-category')).toHaveCount(0);
});
