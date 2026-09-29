/**
 * Spec 1053, story 5: where the music library repair cannot run, it is not there.
 *
 * The stateless build has no accounts, no admins and no cluster access, so the
 * Settings entry must not exist at all — not be disabled, not explain itself —
 * and the rest of Settings must be exactly as before.
 */
import { expect, test } from "@playwright/test";
import { login, resetMock } from "./helpers";

test.beforeEach(async () => {
  await resetMock();
});

test("the stateless build has no music library repair entry, and Settings is otherwise unchanged", async ({
  page,
}) => {
  await login(page);
  await page.getByTestId("tab-settings").click();
  await expect(page.getByTestId("settings-dark-toggle")).toBeVisible();
  await expect(page.getByTestId("settings-music-repair")).toHaveCount(0);
  await expect(page.getByTestId("settings-music-libs")).toHaveCount(0);
});
