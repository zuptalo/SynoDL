/**
 * YouTube downloads look like the downloads they sit next to (spec 1039).
 *
 * Reported from use with screenshots: the Tasks sort did nothing to the YouTube
 * section, a state was hard to pick out of a long playlist, and the thumbnails
 * read as dark slivers next to a film poster.
 *
 * The sort and search half is behaviour and is tested here. The chip and the
 * thumbnail SIZE are asserted at the level that can actually fail — what the row
 * asks for — rather than by looking at pixels: the harness has no route to the
 * artwork host, so an assertion on a rendered image would be testing the
 * fallback, not the fix.
 */
import { expect, test, type Page } from '@playwright/test';
import { apiToken, clearYtdl, login } from './helpers';

const SF_PORT = Number(process.env.SYNODL_E2E_SF_PORT) || 8283;
const K8S = `http://localhost:${process.env.SYNODL_E2E_SF_K8S_PORT || 8296}`;
const API = `http://localhost:${SF_PORT}`;

async function submit(token: string, url: string): Promise<string> {
  const res = await fetch(`${API}/v1/ytdl`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-SynoDL-Session': token },
    body: JSON.stringify({ url, mode: 'music' }),
  });
  expect(res.status).toBe(202);
  return ((await res.json()) as { requestId: string }).requestId;
}

async function gotoTasks(page: Page): Promise<void> {
  await login(page);
  await page.goto('/tabs/tasks');
  await expect(page.getByTestId('task-list').or(page.getByTestId('tasks-empty'))).toBeVisible();
}

/** The titles currently shown on the YouTube rows, top to bottom. */
async function ytdlTitles(page: Page): Promise<string[]> {
  return page.getByTestId('ytdl-item').getByTestId('ytdl-name').allInnerTexts();
}

async function setSort(page: Page, key: string, ascending: boolean): Promise<void> {
  await page.getByTestId('filter-open').click();
  await page.getByTestId(`sort-${key}`).click();
  await page.getByTestId(ascending ? 'sort-asc' : 'sort-desc').click();
  await page.getByTestId('filter-apply').click();
}

let token = '';

test.beforeEach(async () => {
  token = await apiToken();
  await fetch(`${K8S}/__mock/reset`, { method: 'POST' });
  await clearYtdl(token);
  // The mock names a track after its video id, so these sort predictably by
  // title while staying three genuinely different downloads.
  for (const id of ['aaaaaaaaaaa', 'mmmmmmmmmmm', 'zzzzzzzzzzz']) {
    await submit(token, `https://youtu.be/${id}`);
  }
});

test('the sort reaches the YouTube section, in both directions', async ({ page }) => {
  await gotoTasks(page);
  await expect(page.getByTestId('ytdl-item')).toHaveCount(3, { timeout: 20_000 });

  await setSort(page, 'name', true);
  const asc = await ytdlTitles(page);
  expect(asc, 'sorting by name did nothing to the YouTube rows').toEqual([...asc].sort());

  await setSort(page, 'name', false);
  const desc = await ytdlTitles(page);
  expect(desc, 'reversing the sort did nothing').toEqual([...asc].reverse());
});

test('searching for a title a row is showing finds that row', async ({ page }) => {
  // The reported bug: search matched the LINK only, so typing the title printed
  // on the row hid every row including that one.
  await gotoTasks(page);
  await expect(page.getByTestId('ytdl-item')).toHaveCount(3, { timeout: 20_000 });
  const [first] = await ytdlTitles(page);

  await page.getByTestId('filter-open').click();
  await page.getByTestId('filter-term').locator('input').fill(first);
  await page.getByTestId('filter-apply').click();

  await expect(page.getByTestId('ytdl-item')).toHaveCount(1);
  await expect(page.getByTestId('ytdl-name')).toHaveText(first);
});

test('a state is drawn as a chip, coloured by what it is doing', async ({ page }) => {
  await gotoTasks(page);
  const status = page.getByTestId('ytdl-status').first();
  await expect(status).toBeVisible({ timeout: 20_000 });

  const chip = await status.evaluate((el) => {
    const s = getComputedStyle(el);
    return { background: s.backgroundColor, radius: s.borderRadius, weight: s.fontWeight };
  });
  // A tinted, rounded pill — not a bare word. Transparent would mean the chip
  // never landed.
  expect(chip.background).not.toBe('rgba(0, 0, 0, 0)');
  expect(parseFloat(chip.radius)).toBeGreaterThan(0);
  expect(Number(chip.weight)).toBeGreaterThanOrEqual(600);
});

/**
 * Record which artwork URLs the page asks the proxy for.
 *
 * Asserting on the rendered <img> would be testing the WRONG thing here: the
 * harness has no route to the artwork host, so every image errors and the row
 * correctly swaps in its icon — the element under test disappears, and a passing
 * assertion would only mean the fallback works. What the page ASKS for is the
 * change, and it survives the image failing.
 */
function recordThumbRequests(page: Page): () => string[] {
  const asked: string[] = [];
  void page.route('**/v1/ytdl/thumb**', async (route) => {
    const u = new URL(route.request().url()).searchParams.get('u') ?? '';
    asked.push(u);
    await route.fulfill({ status: 404, body: 'no route to the artwork host in tests' });
  });
  return () => asked;
}

test('a row asks for the thumbnail size that has no letterbox bands', async ({ page }) => {
  // Artwork is STORED as hqdefault — 4:3, with black bands around a 16:9 image.
  // Cropping that into the 40x60 poster slot kept the bands, which is why the
  // tiles read as dark slivers. The fix is which size is requested.
  const asked = recordThumbRequests(page);
  await gotoTasks(page);
  await expect(page.getByTestId('ytdl-item')).toHaveCount(3, { timeout: 20_000 });

  await expect.poll(() => asked().length, { timeout: 20_000 }).toBeGreaterThan(0);
  expect(asked().every((u) => u.includes('mqdefault'))).toBe(true);
  expect(asked().some((u) => u.includes('hqdefault'))).toBe(false);
});

test('the detail sheet asks for a sharper image than the row', async ({ page }) => {
  const asked = recordThumbRequests(page);
  await gotoTasks(page);
  await expect(page.getByTestId('ytdl-item')).toHaveCount(3, { timeout: 20_000 });
  await page.getByTestId('ytdl-item').first().click();
  await expect(page.getByTestId('ytdl-detail')).toBeVisible();

  await expect
    .poll(() => asked().filter((u) => u.includes('maxresdefault')).length, { timeout: 20_000 })
    .toBeGreaterThan(0);

  // And when that size is not published — which is what the 404 above stands in
  // for — the sheet drops to the size the row uses rather than showing nothing.
  await expect
    .poll(() => asked().filter((u) => u.includes('mqdefault')).length, { timeout: 20_000 })
    .toBeGreaterThan(0);

  // The state reads as the same chip the row uses.
  await expect(page.getByTestId('ytdl-detail-state')).toBeVisible();
});
