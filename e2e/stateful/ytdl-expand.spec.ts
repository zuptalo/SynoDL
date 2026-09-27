/**
 * A playlist or channel becomes its own items (spec 0013, US6).
 *
 * Spec 0012 fetched a channel as ONE bulk job. That could not say which item was
 * downloading, could not report which item failed, and could not be retried at
 * item granularity — so a channel was all-or-nothing in a way nothing else in
 * the app is.
 *
 * Expansion is driven here through the mock cluster's emit control, so the path
 * under test is the real one: enumeration worker → its pod log → the entries
 * SynoDL parses → one queued download per entry.
 */
import { expect, test, type Page } from "@playwright/test";
import { apiToken, clearYtdl, login } from "./helpers";

const SF_PORT = Number(process.env.SYNODL_E2E_SF_PORT) || 8283;
const K8S = `http://localhost:${process.env.SYNODL_E2E_SF_K8S_PORT || 8296}`;
const API = `http://localhost:${SF_PORT}`;
const ENTRY = "[synodl-entry]";

type Row = {
  requestId: string;
  kind: string;
  state: string;
  title?: string;
  counts?: { total: number };
};

async function submit(
  token: string,
  url: string,
): Promise<{ requestId: string; kind: string }> {
  const res = await fetch(`${API}/v1/ytdl`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-SynoDL-Session": token },
    body: JSON.stringify({ url, mode: "music" }),
  });
  expect(res.status).toBe(202);
  return (await res.json()) as { requestId: string; kind: string };
}

async function rows(token: string): Promise<Row[]> {
  const res = await fetch(`${API}/v1/ytdl?limit=200`, {
    headers: { "X-SynoDL-Session": token },
  });
  return ((await res.json()) as { downloads: Row[] }).downloads;
}

async function items(token: string, groupId: string): Promise<Row[]> {
  const res = await fetch(`${API}/v1/ytdl/${groupId}/items?limit=500`, {
    headers: { "X-SynoDL-Session": token },
  });
  if (!res.ok) return [];
  return ((await res.json()) as { items: Row[] }).items;
}

/** Feed the enumeration worker's output, once the cluster has started it. */
async function emitEntries(groupId: string, ids: string[]): Promise<void> {
  const name = `synodl-ytdl-${groupId}-expand`;
  const deadline = Date.now() + 25_000;
  for (;;) {
    const lines = ids
      .map((id) => `${ENTRY} id=${id} uploader=Lo-fi Beats title=Track ${id}`)
      .join("\n");
    const res = await fetch(`${K8S}/__mock/jobs/${name}/emit`, {
      method: "POST",
      body: lines,
    });
    if (res.ok) {
      // Then let it finish, so the reconciler reads what it printed.
      const done = await fetch(`${K8S}/__mock/jobs/${name}/succeed`, {
        method: "POST",
      });
      if (done.ok) return;
    }
    if (Date.now() > deadline)
      throw new Error("enumeration worker never appeared");
    await new Promise((r) => setTimeout(r, 500));
  }
}

async function gotoTasks(page: Page): Promise<void> {
  await login(page);
  await page.goto("/tabs/tasks");
  await expect(
    page
      .getByTestId("task-list")
      .or(page.getByTestId("ytdl-list"))
      .or(page.getByTestId("upload-list"))
      .or(page.getByTestId("tasks-empty"))
      .first(),
  ).toBeVisible();
}

let token = "";

test.beforeEach(async () => {
  token = await apiToken();
  await fetch(`${K8S}/__mock/reset`, { method: "POST" });
  await clearYtdl(token);
});

test("a channel expands into one download per item", async () => {
  const { requestId, kind } = await submit(
    token,
    "https://www.youtube.com/@lofi",
  );
  expect(kind, "a channel is a group, not a single download").toBe("group");

  await emitEntries(requestId, ["aaaaaaaaaaa", "bbbbbbbbbbb", "ccccccccccc"]);

  await expect
    .poll(() => items(token, requestId).then((i) => i.length), {
      timeout: 30_000,
    })
    .toBe(3);

  // Each item is a download in its own right.
  for (const it of await items(token, requestId)) {
    expect(it.kind).toBe("item");
    expect([
      "queued",
      "scheduled",
      "downloading",
      "completed",
      "failed",
    ]).toContain(it.state);
  }
});

test("the Tasks list gains ONE row for a channel, not one per item", async ({
  page,
}) => {
  // SC-004a. With no ceiling on expansion, a flat list would push every other
  // download off the screen — which is what the group row prevents.
  const { requestId } = await submit(token, "https://www.youtube.com/@lofi");
  await emitEntries(requestId, [
    "aaaaaaaaaaa",
    "bbbbbbbbbbb",
    "ccccccccccc",
    "ddddddddddd",
  ]);
  await expect
    .poll(() => items(token, requestId).then((i) => i.length), {
      timeout: 30_000,
    })
    .toBe(4);

  const top = await rows(token);
  expect(top).toHaveLength(1);
  expect(top[0].requestId).toBe(requestId);
  expect(top[0].kind).toBe("group");

  await gotoTasks(page);
  await expect(page.getByTestId("ytdl-item")).toHaveCount(1);
  // And the row says how its contents are getting on.
  await expect(page.getByTestId("ytdl-group-summary")).toBeVisible();
});

test("opening the group row shows its items", async ({ page }) => {
  const { requestId } = await submit(token, "https://www.youtube.com/@lofi");
  await emitEntries(requestId, ["aaaaaaaaaaa", "bbbbbbbbbbb"]);
  await expect
    .poll(() => items(token, requestId).then((i) => i.length), {
      timeout: 30_000,
    })
    .toBe(2);

  await gotoTasks(page);
  await page.getByTestId("ytdl-item").first().click();
  await expect(page.getByTestId("ytdl-group-items")).toBeVisible();
  await expect(page.getByTestId("ytdl-group-item")).toHaveCount(2);
});

test("re-submitting a channel queues only what is new", async () => {
  // FR-020. Checked at expansion, before anything is queued, so a re-run does
  // not create rows that would immediately finish having done nothing.
  const first = await submit(token, "https://www.youtube.com/@lofi");
  await emitEntries(first.requestId, ["aaaaaaaaaaa", "bbbbbbbbbbb"]);
  await expect
    .poll(() => items(token, first.requestId).then((i) => i.length), {
      timeout: 30_000,
    })
    .toBe(2);

  // Finish them both, so they count as held.
  for (const it of await items(token, first.requestId)) {
    const deadline = Date.now() + 25_000;
    for (;;) {
      const res = await fetch(`${K8S}/__mock/jobs/${it.requestId}/succeed`, {
        method: "POST",
      });
      if (res.ok || Date.now() > deadline) break;
      await new Promise((r) => setTimeout(r, 500));
    }
  }
  await expect
    .poll(
      async () =>
        (await items(token, first.requestId)).filter(
          (i) => i.state === "completed",
        ).length,
      { timeout: 30_000 },
    )
    .toBe(2);

  // Wait for the GROUP itself to finish, not just its items. Re-submitting a
  // channel that is still running is a duplicate and is refused — correctly —
  // so the second submission has to come after the first has settled.
  await expect
    .poll(
      async () =>
        (await rows(token)).find((r) => r.requestId === first.requestId)?.state,
      { timeout: 30_000 },
    )
    .toBe("completed");

  // Same channel again, now listing one extra item.
  const second = await submit(token, "https://www.youtube.com/@lofi");
  await emitEntries(second.requestId, [
    "aaaaaaaaaaa",
    "bbbbbbbbbbb",
    "zzzzzzzzzzz",
  ]);

  await expect
    .poll(() => items(token, second.requestId).then((i) => i.length), {
      timeout: 30_000,
    })
    .toBe(1);
  const fresh = await items(token, second.requestId);
  expect(fresh[0].title).toContain("zzzzzzzzzzz");
});

test("an open group sheet does not refetch its items on every poll", async ({
  page,
}) => {
  // The sheet used to clear and refetch its items every few seconds: its
  // watcher's getter returned a fresh array each run, so Vue's reference
  // comparison fired on every re-render of the polled list.
  //
  // Counting REQUESTS is the instrument, not sampling the rendered rows —
  // the clear-and-refetch window is milliseconds wide, so watching for an
  // empty list misses it and the test passes with the bug present.
  const { requestId } = await submit(
    token,
    "https://www.youtube.com/playlist?list=PLtest",
  );
  await emitEntries(requestId, ["aaaaaaaaaaa", "bbbbbbbbbbb", "ccccccccccc"]);
  await expect
    .poll(() => items(token, requestId).then((i) => i.length), {
      timeout: 30_000,
    })
    .toBe(3);

  await gotoTasks(page);

  let itemFetches = 0;
  page.on("request", (r) => {
    if (/\/v1\/ytdl\/[^/]+\/items/.test(r.url())) itemFetches += 1;
  });

  await page.getByTestId("ytdl-item").first().click();
  await expect(page.getByTestId("ytdl-group-items")).toBeVisible();
  await expect(page.getByTestId("ytdl-group-item")).toHaveCount(3);

  const afterOpen = itemFetches;
  expect(
    afterOpen,
    "opening the sheet should fetch the items once",
  ).toBeGreaterThan(0);

  // Nothing about the group changes for the next 15s — no item finishes, no
  // count moves — so there is nothing to re-read.
  await page.waitForTimeout(15_000);

  const extra = itemFetches - afterOpen;
  expect(
    extra,
    `items were refetched ${extra} more times while nothing changed; the sheet is reloading on every poll`,
  ).toBeLessThanOrEqual(1);

  await expect(page.getByTestId("ytdl-group-item")).toHaveCount(3);
});

test("tapping an item inside a group opens its details, not a gone message", async ({
  page,
}) => {
  // A group's items are deliberately absent from the top-level list, so the
  // detail sheet could never resolve one from there — every item, in every
  // state, reported "This download is no longer available".
  const { requestId } = await submit(
    token,
    "https://www.youtube.com/playlist?list=PLtest",
  );
  await emitEntries(requestId, ["aaaaaaaaaaa", "bbbbbbbbbbb"]);
  await expect
    .poll(() => items(token, requestId).then((i) => i.length), {
      timeout: 30_000,
    })
    .toBe(2);

  await gotoTasks(page);
  await page.getByTestId("ytdl-item").first().click();
  await expect(page.getByTestId("ytdl-group-items")).toBeVisible();

  await page.getByTestId("ytdl-group-item").first().click();

  await expect(page.getByTestId("ytdl-detail")).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.getByTestId("ytdl-detail-gone")).toHaveCount(0);
  // And it carries the item's own facts, including where it came from.
  await expect(page.getByTestId("ytdl-detail-title")).not.toHaveText("—");
  await expect(page.getByTestId("ytdl-detail-url")).toContainText(
    "youtube.com/watch",
  );
  await expect(page.getByTestId("ytdl-detail-group")).toBeVisible();
});

test("a track inside a playlist is rendered like any other download", async ({
  page,
}) => {
  // Spec 1037. The tracks used to be a thinner copy of the row they came from:
  // a title and a state, with no artwork, no source marker, no artist, no mode.
  // They are now rendered by the SAME component, so the two cannot drift apart.
  const { requestId } = await submit(
    token,
    "https://www.youtube.com/playlist?list=PLtest",
  );
  await emitEntries(requestId, ["aaaaaaaaaaa", "bbbbbbbbbbb"]);
  await expect
    .poll(() => items(token, requestId).then((i) => i.length), {
      timeout: 30_000,
    })
    .toBe(2);

  await gotoTasks(page);
  await page.getByTestId("ytdl-item").first().click();
  await expect(page.getByTestId("ytdl-group-items")).toBeVisible();

  const row = page.getByTestId("ytdl-group-item").first();
  await expect(row).toBeVisible();
  // The same fields the top-level row carries.
  await expect(row.getByTestId("ytdl-source")).toHaveText("YouTube");
  await expect(row.getByTestId("ytdl-name")).not.toHaveText("");
  await expect(row.getByTestId("ytdl-status")).toBeVisible();
  // Artwork is asserted at the DATA level in the next test, not here: the row
  // falls back to an icon when an image fails to load, and this harness has no
  // route to the thumbnail host — so a rendered <img> would be testing the
  // network rather than the feature.
});

test("every track carries artwork without a request per item", async () => {
  // FR-002: expansion has no ceiling, so the address is derived from the id.
  const { requestId } = await submit(
    token,
    "https://www.youtube.com/playlist?list=PLtest",
  );
  await emitEntries(requestId, ["aaaaaaaaaaa", "bbbbbbbbbbb", "ccccccccccc"]);
  await expect
    .poll(() => items(token, requestId).then((i) => i.length), {
      timeout: 30_000,
    })
    .toBe(3);

  for (const it of await items(token, requestId)) {
    const row = it as unknown as { artwork?: string; requestId: string };
    expect(row.artwork, `${row.requestId} has no artwork`).toBeTruthy();
    expect(row.artwork).toContain("i.ytimg.com");
  }
});

/** Drive one item's worker, waiting for the reconciler to have started it. */
async function driveItem(requestId: string, action: string): Promise<void> {
  const deadline = Date.now() + 25_000;
  for (;;) {
    const res = await fetch(`${K8S}/__mock/jobs/${requestId}/${action}`, {
      method: "POST",
    });
    if (res.ok) return;
    if (res.status !== 404 || Date.now() > deadline) {
      throw new Error(`drive ${action} failed: ${res.status}`);
    }
    await new Promise((r) => setTimeout(r, 500));
  }
}

async function groupState(token: string, id: string): Promise<string> {
  return (await rows(token)).find((r) => r.requestId === id)?.state ?? "";
}

// Spec 2035. A playlist is what its tracks add up to, always — and its failed
// tracks are one tap to retry, not one swipe each.
test("retrying a playlist's failed tracks in one tap brings the playlist to finished", async ({
  page,
}) => {
  const { requestId: gid } = await submit(
    token,
    "https://www.youtube.com/@lofi",
  );
  await emitEntries(gid, ["aaaaaaaaaaa", "bbbbbbbbbbb"]);
  await expect
    .poll(() => items(token, gid).then((i) => i.length), { timeout: 30_000 })
    .toBe(2);
  const [a, b] = await items(token, gid);

  await driveItem(a.requestId, "fail");
  await driveItem(b.requestId, "succeed");
  await expect
    .poll(() => groupState(token, gid), { timeout: 20_000 })
    .toBe("failed");

  await gotoTasks(page);
  await page.getByTestId("ytdl-item").first().click();
  const retry = page.getByTestId("ytdl-group-retry-failed");
  await expect(retry).toHaveText(/Retry 1 failed/);
  await retry.click();

  // Reopened while the retried track runs — not still claiming to have failed.
  await expect
    .poll(() => groupState(token, gid), { timeout: 20_000 })
    .not.toBe("failed");
  await expect(retry).toBeHidden();

  await driveItem(a.requestId, "succeed");
  await expect
    .poll(() => groupState(token, gid), { timeout: 20_000 })
    .toBe("completed");
  await page.getByTestId("ytdl-group-close").click();
  await expect(page.getByTestId("ytdl-status").first()).toHaveText(/finished/i);
});

// Spec 2035. Tapping a swipe action closes the row again, as a NAS row does.
test("tapping retry on a swiped row slides it closed", async ({ page }) => {
  const { requestId: gid } = await submit(
    token,
    "https://www.youtube.com/@lofi",
  );
  await emitEntries(gid, ["aaaaaaaaaaa"]);
  await expect
    .poll(() => items(token, gid).then((i) => i.length), { timeout: 30_000 })
    .toBe(1);
  const [a] = await items(token, gid);
  await driveItem(a.requestId, "fail");
  await expect
    .poll(() => groupState(token, gid), { timeout: 20_000 })
    .toBe("failed");

  await gotoTasks(page);
  const sliding = page
    .locator("ion-item-sliding")
    .filter({
      has: page.getByTestId("ytdl-item"),
    })
    .first();
  await sliding.evaluate((el) => (el as HTMLIonItemSlidingElement).open("end"));
  await expect
    .poll(() =>
      sliding.evaluate((el) =>
        (el as HTMLIonItemSlidingElement).getOpenAmount(),
      ),
    )
    .toBeGreaterThan(0);

  await sliding.getByTestId("ytdl-retry").click();
  await expect
    .poll(() =>
      sliding.evaluate((el) =>
        (el as HTMLIonItemSlidingElement).getOpenAmount(),
      ),
    )
    .toBe(0);
});

// Spec 1044. A playlist row shows how much of it is saved as a bar, and says
// "downloading" only while a track in it actually is — not while every one of
// its tracks is waiting behind other downloads.
test("a playlist row shows how much is saved, and whether anything is running", async ({
  page,
}) => {
  // Fill every slot first (the e2e stack runs four at once), so the playlist's
  // tracks have to wait their turn.
  const singles: string[] = [];
  for (let i = 0; i < 4; i++) {
    singles.push((await submit(token, `https://youtu.be/busy${i}`)).requestId);
  }
  for (const id of singles) await driveItem(id, "start");

  const { requestId: gid } = await submit(
    token,
    "https://www.youtube.com/@lofi",
  );
  await emitEntries(gid, ["aaaaaaaaaaa", "bbbbbbbbbbb"]);
  await expect
    .poll(() => items(token, gid).then((i) => i.length), { timeout: 30_000 })
    .toBe(2);

  await gotoTasks(page);
  const row = page
    .getByTestId("ytdl-item")
    .filter({ has: page.getByTestId("ytdl-group-summary") });
  await expect(row.getByTestId("ytdl-status")).toHaveText(/Pending/);

  // Free the slots: the playlist's tracks start, and one of them saves.
  for (const id of singles) await driveItem(id, "succeed");
  const [a] = await items(token, gid);
  await driveItem(a.requestId, "succeed");

  await expect(row.getByTestId("ytdl-status")).toHaveText(/Downloading/, {
    timeout: 20_000,
  });
  const bar = row.getByTestId("ytdl-progress");
  await expect
    .poll(() => bar.evaluate((el) => (el as HTMLIonProgressBarElement).value), {
      timeout: 20_000,
    })
    .toBe(0.5);
});

// Spec 1049. Everything failed is one tap to retry from the Tasks menu — the
// count is in tracks, and one playlist's failed tracks are retried through the
// playlist, not one by one.
test("the Tasks menu retries every failed download at once", async ({
  page,
}) => {
  const { requestId: single } = await submit(
    token,
    "https://youtu.be/lonely00001",
  );
  await driveItem(single, "fail");

  const { requestId: gid } = await submit(
    token,
    "https://www.youtube.com/@lofi",
  );
  await emitEntries(gid, ["aaaaaaaaaaa", "bbbbbbbbbbb", "ccccccccccc"]);
  await expect
    .poll(() => items(token, gid).then((i) => i.length), { timeout: 30_000 })
    .toBe(3);
  const [a, b, c] = await items(token, gid);
  await driveItem(a.requestId, "fail");
  await driveItem(b.requestId, "fail");
  await driveItem(c.requestId, "succeed");
  await expect
    .poll(() => groupState(token, gid), { timeout: 20_000 })
    .toBe("failed");
  await expect
    .poll(() => groupState(token, single), { timeout: 20_000 })
    .toBe("failed");

  await gotoTasks(page);
  await page.getByTestId("overflow-open").click();
  const retry = page.getByRole("button", { name: /Retry failed/ });
  await expect(retry).toHaveText(/Retry failed \(3\)/);
  await retry.click();

  // Both the single and the playlist's two tracks are running again; nothing
  // is still "failed" — and the menu now has nothing left to retry.
  await expect
    .poll(() => groupState(token, single), { timeout: 20_000 })
    .not.toBe("failed");
  await expect
    .poll(() => groupState(token, gid), { timeout: 20_000 })
    .not.toBe("failed");
  await expect
    .poll(
      async () =>
        (await items(token, gid)).filter((i) => i.state === "failed").length,
      { timeout: 20_000 },
    )
    .toBe(0);
  await page.getByTestId("overflow-open").click();
  await expect(
    page.getByRole("button", { name: /Retry failed \(0\)/ }),
  ).toBeVisible();
});

// Spec 2038. A tapped notification opens /tabs/tasks?download=<id>. It used to
// open ?task=<id>, the NAS task sheet, which found "no longer available".
test("a download's deep link opens its own sheet, group or single", async ({ page }) => {
  const { requestId: gid } = await submit(token, "https://www.youtube.com/@lofi");
  await emitEntries(gid, ["aaaaaaaaaaa", "bbbbbbbbbbb"]);
  await expect
    .poll(() => items(token, gid).then((i) => i.length), { timeout: 30_000 })
    .toBe(2);
  const [a] = await items(token, gid);

  await login(page);
  await page.goto(`/tabs/tasks?download=${gid}`);
  await expect(page.getByTestId("ytdl-group-items")).toBeVisible();
  await expect(page.getByTestId("ytdl-group-item")).toHaveCount(2);
  await expect(page.getByText("This task is no longer available.")).toHaveCount(0);
  await page.getByTestId("ytdl-group-close").click();

  await page.goto(`/tabs/tasks?download=${a.requestId}`);
  await expect(page.getByTestId("ytdl-detail")).toBeVisible();
  await expect(page.getByTestId("ytdl-detail-gone")).toHaveCount(0);
});

// Spec 2039. The server pages the list; the Tasks screen shows all of it. The
// first assertion proves the paging is real (a bare request is one page of
// fifty with more to come), so the second — every row on screen — means the
// client followed the cursor rather than the fixture fitting in one page.
test("more than a page of downloads are all in the Tasks list", async ({
  page,
}) => {
  for (let i = 0; i < 55; i++) {
    await submit(token, `https://youtu.be/page${String(i).padStart(7, "0")}`);
  }
  const bare = await fetch(`${API}/v1/ytdl`, {
    headers: { "X-SynoDL-Session": token },
  });
  const first = (await bare.json()) as { downloads: Row[]; nextCursor?: string };
  expect(first.downloads).toHaveLength(50);
  expect(first.nextCursor).toBeTruthy();

  await gotoTasks(page);
  await expect(page.getByTestId("ytdl-item")).toHaveCount(55, {
    timeout: 20_000,
  });
});

// Spec 2039. A playlist's sheet shows every track at once, not the first
// hundred and the rest as it scrolls.
test("more than a page of tracks are all in the playlist sheet", async ({
  page,
}) => {
  const { requestId: gid } = await submit(
    token,
    "https://www.youtube.com/@lofi",
  );
  const ids = Array.from(
    { length: 120 },
    (_, i) => `t${String(i).padStart(3, "0")}aaaaaaa`,
  );
  await emitEntries(gid, ids);
  await expect
    .poll(() => items(token, gid).then((i) => i.length), { timeout: 30_000 })
    .toBe(120);

  await gotoTasks(page);
  await page.getByTestId("ytdl-item").first().click();
  await expect(page.getByTestId("ytdl-group-items")).toBeVisible();
  await expect(page.getByTestId("ytdl-group-item")).toHaveCount(120, {
    timeout: 20_000,
  });
});

/** Feed a line of worker output to one item's job, once it exists. */
async function emitTo(requestId: string, line: string): Promise<void> {
  const deadline = Date.now() + 25_000;
  for (;;) {
    const res = await fetch(`${K8S}/__mock/jobs/${requestId}/emit`, {
      method: "POST",
      body: line,
    });
    if (res.ok) return;
    if (res.status !== 404 || Date.now() > deadline) {
      throw new Error(`emit failed: ${res.status}`);
    }
    await new Promise((r) => setTimeout(r, 500));
  }
}

// Spec 1050. A playlist whose only failures are permanent — the video is gone
// from YouTube — can never finish, so the Tasks menu clears those in one go.
// One whose failure was a refusal is NOT among them: that may save next time.
test("the Tasks menu clears playlists that can never finish, and only those", async ({
  page,
}) => {
  const { requestId: gone } = await submit(
    token,
    "https://www.youtube.com/@lofi",
  );
  await emitEntries(gone, ["aaaaaaaaaaa", "bbbbbbbbbbb"]);
  await expect
    .poll(() => items(token, gone).then((i) => i.length), { timeout: 30_000 })
    .toBe(2);
  const [a, b] = await items(token, gone);
  await driveItem(a.requestId, "start");
  await emitTo(a.requestId, "ERROR: [youtube] aaaaaaaaaaa: Video unavailable");
  await driveItem(a.requestId, "fail");
  await driveItem(b.requestId, "succeed");

  const { requestId: refused } = await submit(
    token,
    "https://www.youtube.com/@chill",
  );
  await emitEntries(refused, ["ccccccccccc"]);
  await expect
    .poll(() => items(token, refused).then((i) => i.length), { timeout: 30_000 })
    .toBe(1);
  const [c] = await items(token, refused);
  await driveItem(c.requestId, "start");
  await emitTo(c.requestId, "ERROR: [youtube] ccccccccccc: HTTP Error 429: Too Many Requests");
  await driveItem(c.requestId, "fail");

  await expect
    .poll(() => groupState(token, gone), { timeout: 20_000 })
    .toBe("failed");
  // A refused track is "failed, retrying by itself" (spec 1043), so its
  // playlist does not read as failed — which is the point: it is not hopeless.
  await expect
    .poll(() => items(token, refused).then((i) => i[0]?.state), {
      timeout: 20_000,
    })
    .toBe("failed");

  await gotoTasks(page);
  await page.getByTestId("overflow-open").click();
  const clear = page.getByRole("button", { name: /Clear failed for good/ });
  await expect(clear).toHaveText(/Clear failed for good \(1\)/);
  await clear.click();
  await page.getByRole("button", { name: /^Clear 1$/ }).click();

  // The hopeless one is gone; the refused one is still there to be retried.
  await expect
    .poll(() => rows(token).then((r) => r.map((x) => x.requestId).sort()), {
      timeout: 20_000,
    })
    .toEqual([refused]);
  await expect(page.getByTestId("ytdl-item")).toHaveCount(1);
});
