/**
 * Filtering and sorting YouTube downloads with the same controls as NAS ones
 * (spec 1039).
 *
 * The Tasks list holds two kinds of download and one filter sheet. Until now the
 * sheet only reached one of them: the YouTube section was filtered by hand, by a
 * search term matched against the URL, and never sorted at all — so changing the
 * sort appeared to do nothing to half the screen, and searching for a title a
 * row was visibly showing found nothing.
 *
 * This mirrors `applyTaskFilter` deliberately, rather than trying to make one
 * function serve both. The two have different fields, different states and
 * different things that can be unknown; a shared function would be a pile of
 * conditionals whose only job is remembering which kind it was handed. What must
 * be shared is the FILTER STATE, and it is.
 */
import type { YtdlDownload, YtdlState } from '@/services/api';
import type { TaskFilterState } from '@/services/task-sort';

/**
 * How much is happening, for the status sort: HIGHER means more active, so the
 * sheet's default order — Descending — puts what is doing something on top and
 * what is over at the bottom (spec 1044). It was a rank the other way round,
 * which made "Status" with the default order list finished and failed rows
 * first. The same shape as the NAS ranking, so both sections move together.
 */
const ACTIVITY: Record<YtdlState, number> = {
  downloading: 5,
  scheduled: 4,
  resolving: 3,
  queued: 2,
  completed: 1,
  failed: 0,
};
/** A state this build does not know sits among the waiting ones. */
const UNKNOWN_ACTIVITY = ACTIVITY.queued;

/**
 * A playlist's activity is whether a track in it is running NOW (spec 1044).
 *
 * Its own state reads "downloading" from expansion until its last track
 * finishes — including the hours it spends waiting its turn behind every other
 * queued track — so on state alone every unfinished playlist ties. The server's
 * `active` count is what separates the one being worked on from the ones
 * waiting. A server too old to send it leaves the state to speak for itself.
 */
function activityOf(d: YtdlDownload): number {
  const base = ACTIVITY[d.state] ?? UNKNOWN_ACTIVITY;
  if (d.kind !== 'group' || d.state !== 'downloading') return base;
  const active = d.counts?.active;
  if (active === undefined) return base;
  return active > 0 ? ACTIVITY.downloading : ACTIVITY.queued;
}

/** What a row actually shows, which is what a search should match (FR-002). */
function haystack(d: YtdlDownload): string {
  return [d.title, d.uploader, d.groupName, d.url].filter(Boolean).join(' ').toLowerCase();
}

/**
 * How far along, for the progress sort.
 *
 * A finished download is 1 and a failed one is 0, because "how far did it get?"
 * has an answer for both. A running download with no reading is 0 rather than
 * unknown: it is the least-far-along thing that is running, which is where it
 * belongs in an ordering even though the row correctly shows no bar.
 */
function progressOf(d: YtdlDownload): number {
  if (d.state === 'completed') return 1;
  if (d.counts && d.counts.total > 0) return d.counts.completed / d.counts.total;
  return d.progress ?? 0;
}

/**
 * The value to sort a download by.
 *
 * Several of the sheet's keys — size, peers, speeds, ratio, seeding time — are
 * properties of a NAS transfer that a worker on a cluster simply does not have.
 * Rather than invent a zero for them, which would scramble the section into
 * whatever order the tie-break produced, they all fall back to WHEN IT WAS
 * ASKED FOR. That is the list's own default order, so an unrelated sort leaves
 * the section looking untouched instead of shuffled (FR-003).
 */
function keyOf(d: YtdlDownload, key: TaskFilterState['sortKey']): number | string {
  switch (key) {
    case 'status':
      return activityOf(d);
    case 'name':
      return (d.title || d.url).toLowerCase();
    case 'progress':
      return progressOf(d);
    default:
      return d.submittedAt ?? 0;
  }
}

/** Filter by term, then sort. Never mutates the input. */
export function applyYtdlFilter(
  downloads: YtdlDownload[],
  filter: TaskFilterState,
): YtdlDownload[] {
  const term = (filter.term ?? '').trim().toLowerCase();
  const kept = term ? downloads.filter((d) => haystack(d).includes(term)) : [...downloads];

  const dir = filter.ascending ? 1 : -1;
  return kept.sort((a, b) => {
    const ka = keyOf(a, filter.sortKey);
    const kb = keyOf(b, filter.sortKey);
    if (ka < kb) return -1 * dir;
    if (ka > kb) return 1 * dir;
    // Stable tie-break: newest first, matching what the NAS list does and what
    // the server already returns.
    const ta = a.submittedAt ?? 0;
    const tb = b.submittedAt ?? 0;
    if (ta !== tb) return tb - ta;
    return a.requestId < b.requestId ? 1 : -1;
  });
}

/**
 * The order of a playlist's tracks in its sheet (spec 1051): failed first, then
 * in progress, then waiting, then finished — the Tasks list's own order with
 * failed brought to the top. In the list a failed row belongs at the bottom
 * because it is over; inside a playlist the failed tracks are the only ones
 * there is anything to do about, and they used to sit under a few hundred
 * saved ones. Stable within each band, so the queue order the server gave is
 * kept where it still means something.
 */
export function sortPlaylistItems<T extends { state: YtdlState }>(items: readonly T[]): T[] {
  const band = (s: YtdlState): number => {
    switch (s) {
      case 'failed':
        return 0;
      case 'downloading':
      case 'scheduled':
      case 'resolving':
        return 1;
      case 'queued':
        return 2;
      default:
        return 3; // completed
    }
  };
  return items
    .map((item, i) => ({ item, i, b: band(item.state) }))
    .sort((x, y) => x.b - y.b || x.i - y.i)
    .map((x) => x.item);
}
