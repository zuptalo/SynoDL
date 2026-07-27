/** US5 — Filter and sort: sheet, sort keys, term, status multi-select, persistence. */
import { expect, test } from '@playwright/test';
import { login, resetMock, seedTasks } from './helpers';

test.beforeEach(async () => {
  await resetMock();
  await seedTasks([
    { name: 'small-old.iso', status: 'finished', size: 100, downloaded: 100, createdAt: 1000 },
    { name: 'big-new.mkv', status: 'downloading', size: 9000, downloaded: 450, rate: 0, createdAt: 3000 },
    { name: 'medium-paused.zip', status: 'paused', size: 5000, downloaded: 2500, createdAt: 2000 },
  ]);
});

test('sort by size in both directions', async ({ page }) => {
  await login(page);
  // Default sort: creation date, newest first.
  await expect(page.getByTestId('task-item').first()).toContainText('big-new.mkv');

  await page.getByTestId('filter-open').click();
  await page.getByTestId('sort-size').click();
  await page.getByTestId('sort-asc').click();
  await page.getByTestId('filter-apply').click();
  await expect(page.getByTestId('task-item').first()).toContainText('small-old.iso');

  await page.getByTestId('filter-open').click();
  await page.getByTestId('sort-desc').click();
  await page.getByTestId('filter-apply').click();
  await expect(page.getByTestId('task-item').first()).toContainText('big-new.mkv');
});

test('the top search bar filters the download list by name', async ({ page }) => {
  await login(page);
  await expect(page.getByTestId('task-item')).toHaveCount(3);
  await page.getByTestId('tasks-search').locator('input').fill('paused');
  await expect(page.getByTestId('task-item')).toHaveCount(1);
  await expect(page.getByTestId('task-item')).toContainText('medium-paused.zip');
  await page.getByTestId('tasks-search').locator('input').fill('');
  await expect(page.getByTestId('task-item')).toHaveCount(3);
});

test('term filter narrows by name, case-insensitively', async ({ page }) => {
  await login(page);
  await page.getByTestId('filter-open').click();
  await page.getByTestId('filter-term').locator('input').fill('PAUSED');
  await page.getByTestId('filter-apply').click();
  await expect(page.getByTestId('task-item')).toHaveCount(1);
  await expect(page.getByTestId('task-item')).toContainText('medium-paused.zip');
});

test('unchecking a status hides those tasks until re-checked', async ({ page }) => {
  await login(page);
  await expect(page.getByTestId('task-item')).toHaveCount(3);
  await page.getByTestId('filter-open').click();
  await page.getByTestId('status-finished').click();
  await page.getByTestId('filter-apply').click();
  await expect(page.getByTestId('task-item')).toHaveCount(2);
  await expect(page.getByTestId('task-item').filter({ hasText: 'small-old.iso' })).toHaveCount(0);
});

test('filter and sort choices survive a reload', async ({ page }) => {
  await login(page);
  await page.getByTestId('filter-open').click();
  await page.getByTestId('sort-size').click();
  await page.getByTestId('sort-asc').click();
  await page.getByTestId('status-finished').click();
  await page.getByTestId('filter-apply').click();
  await expect(page.getByTestId('task-item').first()).toContainText('medium-paused.zip');

  await page.reload();
  // Same choices in effect: finished hidden, smallest remaining first.
  await expect(page.getByTestId('task-item')).toHaveCount(2);
  await expect(page.getByTestId('task-item').first()).toContainText('medium-paused.zip');
});
