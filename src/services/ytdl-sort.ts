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
 * Lifecycle order for the status sort: the ones doing something first, the ones
 * that are over last — the same shape as the NAS ranking, so sorting by status
 * moves both sections the same way.
 */
const STATE_RANK: Record<YtdlState, number> = {
  downloading: 0,
  scheduled: 1,
  resolving: 2,
  queued: 3,
  completed: 4,
  failed: 5,
};

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
      return STATE_RANK[d.state] ?? 3;
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
