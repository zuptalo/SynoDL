/**
 * Pure client-side task filtering + sorting (spec 0001 US5). The NAS list is
 * small enough that this is plain array work; keeping it out of the proxy
 * keeps the server stateless and this logic unit-testable (FR-018). On the
 * vitest coverage gate.
 */
import type { Task } from '@/types/task';

export type SortKey =
  | 'createdAt'
  | 'status'
  | 'size'
  | 'peers'
  | 'downloadSpeed'
  | 'uploadSpeed'
  | 'name'
  | 'ratio'
  | 'progress'
  | 'remaining';

export interface TaskFilterState {
  term: string;
  sortKey: SortKey;
  ascending: boolean;
  /** Enabled DSM statuses. A status NOT in ALL_STATUSES (new DSM version) is
   *  always shown — the filter only ever hides what the user explicitly saw
   *  and unchecked. */
  statuses: string[];
}

/** The twelve DSM task statuses, in the order the filter sheet lists them
 *  (mirrors the reference app). */
export const ALL_STATUSES = [
  'finished',
  'extracting',
  'finishing',
  'hash_checking',
  'downloading',
  'paused',
  'stopped',
  'waiting',
  'filehosting_waiting',
  'moving',
  'seeding',
  'error',
] as const as unknown as string[];

/** Lifecycle order for the status sort: active work first, terminal states
 *  last. Unknown statuses slot in the middle rather than vanishing. */
const STATUS_RANK: Record<string, number> = {
  downloading: 0,
  seeding: 1,
  extracting: 2,
  finishing: 3,
  hash_checking: 4,
  moving: 5,
  waiting: 6,
  filehosting_waiting: 7,
  paused: 8,
  stopped: 9,
  finished: 11,
  error: 12,
};
const UNKNOWN_STATUS_RANK = 10;

export function defaultTaskFilter(): TaskFilterState {
  return { term: '', sortKey: 'createdAt', ascending: false, statuses: [...ALL_STATUSES] };
}

function ratioOf(t: Task): number {
  return t.downloaded > 0 ? t.uploaded / t.downloaded : 0;
}

function progressOf(t: Task): number {
  return t.size > 0 ? t.downloaded / t.size : 0;
}

/** Remaining seconds, or Infinity when there is no meaningful estimate
 *  (no speed, or size still unknown). */
function remainingOf(t: Task): number {
  if (t.size <= 0 || t.downloadSpeed <= 0) return Infinity;
  return (t.size - t.downloaded) / t.downloadSpeed;
}

function keyOf(t: Task, key: SortKey): number | string {
  switch (key) {
    case 'createdAt':
      return t.createdAt;
    case 'status':
      return STATUS_RANK[t.status] ?? UNKNOWN_STATUS_RANK;
    case 'size':
      return t.size;
    case 'peers':
      return t.peers;
    case 'downloadSpeed':
      return t.downloadSpeed;
    case 'uploadSpeed':
      return t.uploadSpeed;
    case 'name':
      return t.name.toLowerCase();
    case 'ratio':
      return ratioOf(t);
    case 'progress':
      return progressOf(t);
    case 'remaining':
      return remainingOf(t);
  }
}

/** Filter by term + statuses, then sort. Never mutates the input. */
export function applyTaskFilter(tasks: Task[], filter: TaskFilterState): Task[] {
  const term = filter.term.trim().toLowerCase();
  const enabled = new Set(filter.statuses);
  const kept = tasks.filter((t) => {
    if (term && !t.name.toLowerCase().includes(term)) return false;
    // Only statuses the sheet knows about can be toggled off; anything newer
    // than this build always shows (never silently hide a user's task).
    if (!enabled.has(t.status) && ALL_STATUSES.includes(t.status)) return false;
    return true;
  });

  const dir = filter.ascending ? 1 : -1;
  return kept.sort((a, b) => {
    const ka = keyOf(a, filter.sortKey);
    const kb = keyOf(b, filter.sortKey);
    // "No estimate" is not a magnitude: unknown remaining time stays at the
    // bottom in BOTH directions instead of topping the descending sort.
    if (filter.sortKey === 'remaining') {
      const aInf = ka === Infinity;
      const bInf = kb === Infinity;
      if (aInf !== bInf) return aInf ? 1 : -1;
      if (aInf && bInf) return a.id < b.id ? -1 : 1;
    }
    if (ka < kb) return -1 * dir;
    if (ka > kb) return 1 * dir;
    return a.id < b.id ? -1 : a.id > b.id ? 1 : 0; // stable tie-break by id
  });
}
