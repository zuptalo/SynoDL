/**
 * Spec 1055: the stateless build has no accounts and no admins, so it has no
 * YouTube sign-in entry either — absent, not disabled — and Settings is otherwise
 * exactly as before.
 */
import { expect, test } from "@playwright/test";
import { login, resetMock } from "./helpers";

test.beforeEach(async () => {
  await resetMock();
});

test("the stateless build has no YouTube sign-in entry", async ({ page }) => {
  await login(page);
  await page.getByTestId("tab-settings").click();
  await expect(page.getByTestId("settings-dark-toggle")).toBeVisible();
  await expect(page.getByTestId("settings-youtube-signin")).toHaveCount(0);
});
