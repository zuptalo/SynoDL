/** US1 — Connect to my NAS: auth errors, OTP flow, expiry bounce (spec 0001). */
import { expect, test } from '@playwright/test';
import { login, resetMock } from './helpers';

test.beforeEach(async () => {
  await resetMock();
});

test('wrong password shows a distinct plain-language error', async ({ page }) => {
  await page.goto('/');
  await page.getByTestId('login-account').locator('input').fill('admin');
  await page.getByTestId('login-password').locator('input').fill('nope');
  await page.getByTestId('login-submit').click();
  await expect(page.getByTestId('login-error')).toHaveText('Wrong account or password.');
  await expect(page).toHaveURL(/\/login/);
});

test('OTP flow: field appears on demand, wrong code distinct, right code succeeds', async ({ page }) => {
  await page.goto('/');
  await page.getByTestId('login-account').locator('input').fill('otpuser');
  await page.getByTestId('login-password').locator('input').fill('secret');
  await page.getByTestId('login-submit').click();

  // The NAS demanded 2FA: its own message + the code field appears.
  await expect(page.getByTestId('login-error')).toHaveText(
    'This account needs a 2-step verification code.',
  );
  const otp = page.getByTestId('login-otp').locator('input');
  await expect(otp).toBeVisible();

  await otp.fill('999999');
  await page.getByTestId('login-submit').click();
  await expect(page.getByTestId('login-error')).toHaveText(
    'That verification code was not accepted.',
  );

  await otp.fill('000000');
  await page.getByTestId('login-submit').click();
  await expect(page).toHaveURL(/\/tabs\/tasks/);
});

test('an expired NAS session bounces back to login on the next poll', async ({ page }) => {
  await login(page);
  await expect(page.getByTestId('task-item').first()).toBeVisible();
  // Reset drops every mock session: the sid the app holds is now dead. The
  // next poll gets 401 "session" and the app must return to login (no dead UI).
  await resetMock();
  await expect(page).toHaveURL(/\/login/, { timeout: 15_000 });
});
