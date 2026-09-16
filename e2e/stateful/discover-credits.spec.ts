/**
 * Who made a title (spec 0014).
 *
 * Both fake sources publish people, in the two shapes the real ones do: one
 * hands over a cast with character names and IMDb ids in its API, the other
 * names people on the page and keeps their identity on a page of their own.
 *
 * The property every test here is arranged around is that the NAMES are free and
 * the FACES are not. So the first test runs with the fallback switched off, and
 * the whole feature has to still work.
 */
import { expect, test } from "@playwright/test";
import {
  addSource,
  apiTitle,
  apiToken,
  apiSearch,
  clearSources,
  gotoDiscover,
  imdbHits,
  login,
  setIMDb,
  setSourceState,
} from "./helpers";

let token = "";

test.beforeEach(async () => {
  token = await apiToken();
  await clearSources(token);
  await setSourceState("reset");
  await setIMDb("reset");
});

/** The id of the first title a source offers. */
async function firstTitleID(): Promise<string> {
  const items = await apiSearch(token, { page: 1 });
  expect(items.length, "the fake source offered no titles").toBeGreaterThan(0);
  return items[0].id;
}

test("the API-backed source names its cast, characters and crew", async () => {
  // With the fallback down, so this proves the names cost nothing.
  await setIMDb("down");
  await addSource(token, "Mock 30nama", 0, "30nama");
  const detail = await apiTitle(token, await firstTitleID());

  expect(detail.cast?.length, `cast was: ${JSON.stringify(detail.cast)}`).toBe(3);
  expect(detail.cast?.[0].name).toBe("Mock Star");
  // Only this source publishes the part somebody plays.
  expect(detail.cast?.[0].character).toBe("The Lead");
  expect(detail.cast?.[0].imdbId).toBe("nm0000101");
  expect(detail.directors?.[0].name).toBe("Mock Director");
  expect(detail.writers?.[0].name).toBe("Mock Writer");
  // A role nobody is in is ABSENT, never an empty array — the sheet renders a
  // heading for a role that is present.
  expect(detail.creators).toBeUndefined();
});

test("the provider's own stand-in image is never forwarded as a photograph", async () => {
  await addSource(token, "Mock 30nama", 0, "30nama");
  const detail = await apiTitle(token, await firstTitleID());

  // The first cast member has a real picture on the source.
  expect(detail.cast?.[0].photoUrl).toContain("/person-101.jpg");
  // The second's is the /none/ stand-in, which must be dropped so the fallback
  // is asked for the real one instead of a grey silhouette being shown as them.
  expect(detail.cast?.[1].photoUrl ?? "").toBe("");
  // Crew never carry a picture on this source at all.
  expect(detail.directors?.[0].photoUrl ?? "").toBe("");
});

test("the HTML source names its people and resolves who they are", async () => {
  await addSource(token, "Mock ZarFilm", 0);
  const detail = await apiTitle(token, await firstTitleID());

  expect(detail.cast?.map((p) => p.name)).toEqual(["Mock Star", "Mock Nameless"]);
  expect(detail.directors?.[0].name).toBe("Mock Director");
  // This site publishes no character names, and inventing one would be worse
  // than the gap.
  expect(detail.cast?.[0].character).toBeUndefined();

  // Their identity lives on their own page, and one fetch of it buys both the
  // id and a portrait.
  expect(detail.cast?.[0].imdbId).toBe("nm0000101");
  expect(detail.cast?.[0].photoUrl).toContain("/wp-content/uploads/");

  // The director's page serves the THEME's silhouette, which is not a
  // photograph of anybody: it must be dropped, leaving the fallback to find the
  // real one.
  expect(detail.directors?.[0].imdbId).toBe("nm0000201");
  expect(detail.directors?.[0].photoUrl ?? "").toBe("");

  // And somebody whose page carries no IMDb link stays a name.
  expect(detail.cast?.[1].imdbId ?? "").toBe("");

  // The download options — the point of the sheet — are untouched by any of it.
  expect(detail.qualities?.length ?? 0).toBeGreaterThan(0);
});

test("a face is looked up once, however many titles the person is in", async () => {
  await addSource(token, "Mock 30nama", 0, "30nama");
  const items = await apiSearch(token, { page: 1 });
  expect(items.length).toBeGreaterThan(1);

  // Every mock title shares a cast, which is the situation the cache exists for:
  // an evening of browsing hits the same handful of people over and over.
  const photo = async (id: string) => {
    const res = await fetch(
      `http://localhost:${process.env.SYNODL_E2E_SF_PORT ?? 8283}/v1/source/person/${id}/photo`,
    );
    return res.status;
  };

  expect(await photo("nm0000201")).toBe(200);
  const afterFirst = await imdbHits();
  expect(afterFirst, "the first ask should have reached IMDb").toBeGreaterThan(0);

  for (let i = 0; i < 5; i++) expect(await photo("nm0000201")).toBe(200);
  expect(
    await imdbHits(),
    "the same person was looked up again",
  ).toBe(afterFirst);
});

test("a person IMDb has no picture of is a 404, not an error", async () => {
  const port = process.env.SYNODL_E2E_SF_PORT ?? 8283;
  // nm0000999 is the person the fake IMDb knows of but has no photograph of.
  const res = await fetch(`http://localhost:${port}/v1/source/person/nm0000999/photo`);
  expect(res.status).toBe(404);

  // And anything that is not a person id never gets as far as a lookup.
  for (const bad of ["tt2948372", "nm12", "nmzzzzzz"]) {
    const bad404 = await fetch(`http://localhost:${port}/v1/source/person/${bad}/photo`);
    expect(bad404.status, `${bad} should be refused`).toBe(400);
  }
});

test("with IMDb unreachable, everything except the faces is unchanged", async () => {
  await setIMDb("down");
  await addSource(token, "Mock 30nama", 0, "30nama");
  const detail = await apiTitle(token, await firstTitleID());

  expect(detail.cast?.length).toBe(3);

  const port = process.env.SYNODL_E2E_SF_PORT ?? 8283;
  const res = await fetch(`http://localhost:${port}/v1/source/person/nm0000102/photo`);
  // A refusal is the same ordinary 404 a missing photograph is — never a 5xx,
  // and never something a user is shown.
  expect(res.status).toBe(404);
});

test("the sheet shows the people below the download options", async ({ page }) => {
  // The HTML source, because its fake serves real download options — which is
  // what this test is positioning the people against.
  await addSource(token, "Mock ZarFilm", 0);
  await login(page);
  await gotoDiscover(page);

  const cards = page.getByTestId("catalog-card");
  await expect(cards.first()).toBeVisible({ timeout: 30_000 });
  await cards.first().click();

  const credits = page.getByTestId("title-credits");
  await expect(credits).toBeVisible({ timeout: 15_000 });
  await expect(credits).toContainText("Cast");
  await expect(credits).toContainText("Mock Star");
  await expect(credits).toContainText("Director");
  await expect(credits).toContainText("Mock Director");
  // Somebody whose identity could not be resolved still gets a tile with their
  // name on it — a name is worth more than nothing.
  await expect(credits).toContainText("Mock Nameless");

  // Below the downloads, so sending one stays the sheet's first action.
  const optionsBox = await page.locator(".quality-row").first().boundingBox();
  const creditsBox = await credits.boundingBox();
  expect(optionsBox && creditsBox).toBeTruthy();
  expect(creditsBox!.y).toBeGreaterThan(optionsBox!.y);
});
