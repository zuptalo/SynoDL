/**
 * Where a NAS download came from (spec 1042).
 *
 * The Tasks list holds four kinds of row and used to label only two: uploads and
 * YouTube downloads had headings, and then everything from the NAS simply began
 * — a film sent from Discover sitting under a YouTube playlist with nothing
 * between them.
 *
 * There is no origin field to read. Catalog metadata IS the marker: a download
 * carries a catalog id only because it was sent from Discover, which is what
 * "Open in Discover" already relies on. Reading it that way rather than adding a
 * field keeps the two from ever disagreeing.
 */
import type { Task } from '@/types/task';

export type TaskOrigin = 'discover' | 'direct';

/**
 * Whether this download was sent from Discover.
 *
 * The catalog id is the honest test. A title or a poster can be absent from a
 * Discover download whose lookup was thin, and a media type is set from the
 * catalog too — but the id is the one field that exists for no other reason.
 */
export function originOf(t: Task): TaskOrigin {
  return t.catalogId ? 'discover' : 'direct';
}

/**
 * Split a list into its two origins, preserving order within each.
 *
 * Order is preserved rather than re-sorted because the caller has already
 * sorted: the filter sheet decides the order and these sections must not quietly
 * apply a second one (FR-004).
 */
export function splitByOrigin(tasks: Task[]): { discover: Task[]; direct: Task[] } {
  const discover: Task[] = [];
  const direct: Task[] = [];
  for (const t of tasks) {
    if (originOf(t) === 'discover') discover.push(t);
    else direct.push(t);
  }
  return { discover, direct };
}
