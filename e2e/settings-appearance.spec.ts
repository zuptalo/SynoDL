/** Spec 1002 — the dark/light theme toggle (dark default, no system option). */
import { expect, test } from '@playwright/test';
import { login, resetMock } from './helpers';

test.beforeEach(async () => {
  await resetMock();
});

test('dark mode is the default and the toggle switches and persists the palette', async ({ page }) => {
  await login(page);
  await page.getByTestId('tab-settings').click();

  // Default is dark: Ionic's dark palette class is on <html>.
  await expect(page.locator('html')).toHaveClass(/ion-palette-dark/);

  // Toggle to light.
  await page.getByTestId('settings-dark-toggle').click();
  await expect(page.locator('html')).not.toHaveClass(/ion-palette-dark/);

  // The choice survives a reload (persisted for the pre-paint).
  await page.reload();
  await expect(page.locator('html')).not.toHaveClass(/ion-palette-dark/);
});
