/**
 * What a client does with a live update (spec 1038).
 *
 * Pure and separate from the composable that calls it, because these are the
 * rules that decide what a reader sees and they are easier to be sure of when
 * they can be stated as a table rather than watched in a browser.
 *
 * The governing constraint is FR-004a: history is unbounded and the list is
 * paged, so an update is NOT an instruction to go and load a page. A client
 * merges what it is already holding and ignores the rest.
 *
 * The one exception is a genuinely new top-level download. Without it, an admin
 * watching would never see somebody else's submission appear once the poll is
 * gone — so the server marks those `created`, and the list being ordered
 * newest-first is what makes the top the right place to put one.
 */
import type { YtdlDownload, YtdlUpdate } from '@/services/api';

/**
 * Apply an update to a list of downloads, returning a NEW list.
 *
 * `held` is what the caller currently shows. `nested` says whether this list
 * holds a group's items (where nothing is ever inserted, because the sheet is
 * showing one group's contents and a stray row would belong to another) or the
 * top-level list (where a created row is inserted).
 */
export function mergeYtdlUpdate(
  held: YtdlDownload[],
  update: YtdlUpdate,
  nested = false,
): YtdlDownload[] {
  const removed = new Set(update.removed ?? []);
  // A row can be in `created` and still be held — a retried download re-enters
  // the unfinished set and looks new to the server, which has forgotten it.
  // Keyed by id, so both lists merge the same way and neither can duplicate.
  const incoming = new Map<string, YtdlDownload>();
  for (const d of update.changed ?? []) incoming.set(d.requestId, d);
  for (const d of update.created ?? []) incoming.set(d.requestId, d);

  const out: YtdlDownload[] = [];
  const seen = new Set<string>();
  for (const d of held) {
    if (removed.has(d.requestId)) continue;
    const next = incoming.get(d.requestId);
    out.push(next ?? d);
    seen.add(d.requestId);
  }

  // Anything created that was NOT already held goes to the top — but only in
  // the top-level list, and only if it is not somebody's item. An item reaches
  // a client only while that group's sheet is open, and then it is already
  // held; an unheld one belongs to a group nobody is looking at.
  if (!nested) {
    const fresh = (update.created ?? []).filter(
      (d) => !seen.has(d.requestId) && !d.parentId && !removed.has(d.requestId),
    );
    if (fresh.length) return [...fresh, ...out];
  }
  return out;
}

/**
 * Whether an update says anything about this group's items.
 *
 * Used to skip work on the open sheet: most updates are about something else
 * entirely, and a group of several hundred tracks should not be re-rendered
 * because an unrelated download moved.
 */
export function updateTouchesGroup(update: YtdlUpdate, groupId: string): boolean {
  const rows = [...(update.created ?? []), ...(update.changed ?? [])];
  if (rows.some((d) => d.parentId === groupId || d.requestId === groupId)) return true;
  return (update.removed ?? []).length > 0;
}
