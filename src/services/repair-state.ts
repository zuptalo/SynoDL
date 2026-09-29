/**
 * What the music library repair screen offers and how it words things (spec 1053).
 *
 * Pure on purpose. The interesting decisions — what may be offered, what a number
 * means, how a state reads — live here so they can be tested without a browser and
 * so the component stays thin. Where the SERVER already decided something (whether a
 * plan can be applied, continued or undone) this module follows it and never
 * re-derives it: two copies of a safety rule drift, and the server's is the one that
 * is enforced.
 */
import type {
  RepairCheck,
  RepairLeftAlone,
  RepairPhase,
  RepairPlan,
  RepairProgress,
  RepairSnapshot,
  RepairState,
} from '@/services/api';

// ---- numbers ---------------------------------------------------------------------

export function formatCount(n: number): string {
  if (!Number.isFinite(n) || n < 0) return '0';
  return Math.floor(n).toLocaleString('en-US');
}

const UNITS = ['B', 'kB', 'MB', 'GB', 'TB', 'PB'];

/** Decimal units, matching what a NAS and a disk label report. */
export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return '0 B';
  let i = 0;
  let v = n;
  while (v >= 1000 && i < UNITS.length - 1) {
    v /= 1000;
    i++;
  }
  if (i === 0) return `${Math.floor(v)} B`;
  return `${v.toFixed(1)} ${UNITS[i]}`;
}

// ---- progress --------------------------------------------------------------------

const STAGES: Record<RepairPhase, string> = {
  scan: 'Reading the library',
  lookup: 'Looking songs up',
  plan: 'Working out what to change',
  apply: 'Applying the changes',
  restore: 'Putting things back',
};

export function stageLabel(phase: RepairPhase): string {
  return STAGES[phase];
}

/** A percentage only when the worker knows the total; otherwise the bar is indeterminate. */
export function progressPercent(p: RepairProgress | undefined | null): number | null {
  if (!p || !(p.total > 0)) return null;
  return Math.max(0, Math.min(100, Math.floor((p.done / p.total) * 100)));
}

export function progressText(p: RepairProgress | undefined | null): string {
  if (!p) return 'Starting…';
  if (p.total > 0) return `${stageLabel(p.phase)}: ${formatCount(p.done)} of ${formatCount(p.total)}`;
  return stageLabel(p.phase);
}

// ---- time ------------------------------------------------------------------------

/** How long a finished check can still be applied (the server enforces the same 24 h). */
export function expiryText(plan: Pick<RepairPlan, 'expiresAt'>, now: number): string {
  const left = plan.expiresAt - now;
  if (left <= 0) return 'Out of date: run the check again';
  const hours = Math.floor(left / 3600);
  if (hours >= 1) return `Good for ${hours} more hour${hours === 1 ? '' : 's'}`;
  const minutes = Math.floor(left / 60);
  if (minutes >= 1) return `Good for ${minutes} more minute${minutes === 1 ? '' : 's'}`;
  return 'Good for less than a minute';
}

export function relativeTime(ts: number, now: number): string {
  const d = now - ts;
  if (d < 60) return 'just now';
  if (d < 3600) return `${Math.floor(d / 60)} min ago`;
  if (d < 86400) return `${Math.floor(d / 3600)} h ago`;
  if (d < 2 * 86400) return 'yesterday';
  return new Date(ts * 1000).toLocaleDateString('en-US', { day: 'numeric', month: 'short', year: 'numeric' });
}

// ---- the result --------------------------------------------------------------------

export interface ResultRow {
  key: string;
  label: string;
  value: string;
  hint?: string;
  /** Worth drawing attention to (space is short). */
  warn?: boolean;
}

/** The tool refuses to start when it would need more than free space plus 5%. */
const SPACE_MARGIN = 1.05;

export function checkRows(c: RepairCheck): ResultRow[] {
  const enough = c.freeBytes <= 0 || c.bytesNeeded * SPACE_MARGIN <= c.freeBytes;
  return [
    { key: 'duplicates', label: 'Duplicates set aside in .trash', value: formatCount(c.duplicates),
      hint: 'The same song downloaded more than once. One copy is kept.' },
    { key: 'moves', label: 'Files moved', value: formatCount(c.moves) },
    { key: 'retags', label: 'Files retagged', value: formatCount(c.retags) },
    { key: 'playlists', label: 'Playlist files written', value: formatCount(c.playlists) },
    { key: 'conflicts', label: 'Conflicts', value: formatCount(c.conflicts),
      hint: 'Two things wanting one file name; neither is moved.' },
    { key: 'matched', label: 'Songs matched', value: formatCount(c.matched),
      hint: 'Album, track number, year and cover come from a public source.' },
    { key: 'noMatch', label: 'No confident match', value: formatCount(c.noMatch),
      hint: 'Filed without an invented album.' },
    { key: 'notLookedUp', label: 'Could not be looked up', value: formatCount(c.notLookedUp),
      hint: 'A source was unreachable; the next check tries again.' },
    { key: 'toSingles', label: 'Filed in Singles', value: formatCount(c.toSingles) },
    { key: 'reclaimed', label: 'Space freed when .trash is emptied', value: formatBytes(c.bytesReclaimed) },
    { key: 'space', label: 'Space',
      value: c.freeBytes > 0
        ? `${formatBytes(c.bytesNeeded)} needed, ${formatBytes(c.freeBytes)} free`
        : `${formatBytes(c.bytesNeeded)} needed`,
      warn: !enough,
      hint: enough ? undefined : 'Not enough free space: applying would be refused.' },
  ];
}

export interface LeftAloneView {
  total: number;
  groups: { reason: string; count: number }[];
  examples: { path: string; reason: string }[];
  /** How many of `total` the examples do not show. */
  hidden: number;
}

export function leftAloneView(la: RepairLeftAlone): LeftAloneView {
  return {
    total: la.total,
    groups: la.byReason,
    examples: la.examples,
    hidden: Math.max(0, la.total - la.examples.length),
  };
}

/** What the confirmation says will happen — only what is true and non-zero. */
export function confirmLines(c: RepairCheck): string[] {
  const out: string[] = [];
  if (c.duplicates > 0) out.push(`${formatCount(c.duplicates)} duplicate songs will be moved to a .trash folder.`);
  if (c.moves > 0) out.push(`${formatCount(c.moves)} files will be moved and renamed.`);
  if (c.retags > 0) out.push(`${formatCount(c.retags)} files will get clean titles, and album details where a source is sure.`);
  if (c.playlists > 0) out.push(`${formatCount(c.playlists)} playlist files will be written.`);
  out.push('Nothing is deleted: anything removed goes to .trash on the share, which you can empty later.');
  out.push('You can undo this afterwards from this screen.');
  return out;
}

// ---- what to offer ---------------------------------------------------------------

export interface Offers {
  mode: 'unavailable' | 'running' | 'idle';
  canCheck: boolean;
  canApply: boolean;
  canContinue: boolean;
  canUndo: boolean;
  showPlan: boolean;
}

export function offers(s: RepairSnapshot, _now: number): Offers {
  if (!s.available) {
    return { mode: 'unavailable', canCheck: false, canApply: false, canContinue: false, canUndo: false, showPlan: false };
  }
  if (s.current) {
    return { mode: 'running', canCheck: false, canApply: false, canContinue: false, canUndo: false, showPlan: false };
  }
  return {
    mode: 'idle',
    canCheck: true,
    canApply: s.plan?.canApply ?? false,
    canContinue: s.plan?.canContinue ?? false,
    canUndo: s.undo?.canUndo ?? false,
    showPlan: s.plan != null,
  };
}

export function unavailableText(reason: string): string {
  switch (reason) {
    case 'not_configured':
      return 'Repairing the music library needs SynoDL to run in your cluster with a music library configured. It is not set up here.';
    case 'no_image':
      return 'SynoDL could not work out which image it is running, so it cannot start the repair. Its Role must be able to read its own pod, or MUSIC_REPAIR_IMAGE must be set.';
    default:
      return 'The music library repair is not available on this installation.';
  }
}

// ---- words ------------------------------------------------------------------------

const STATE_LABELS: Record<RepairState, string> = {
  running: 'Running',
  finished: 'Finished',
  refused: 'Refused',
  unfinished: 'Did not finish',
};

export function stateLabel(state: RepairState): string {
  return STATE_LABELS[state];
}

const ERRORS: Record<string, string> = {
  busy: 'A repair is already running. It has to finish first.',
  no_plan: 'This server has no such plan. Run a check first.',
  plan_expired: 'That check is more than 24 hours old. Run it again.',
  already_applied: 'That plan was already applied.',
  not_undoable: 'There is nothing to undo for that plan.',
  snapshot_required: 'Confirm that you have taken a snapshot of the music share first.',
  unavailable: 'The music library repair is not available on this installation.',
  start_failed: 'The repair could not be started. Nothing was changed.',
  permission: 'Only an administrator can do that.',
  bad_request: 'That request was not understood.',
};

export function errorMessage(code: string): string {
  return ERRORS[code] ?? 'Something went wrong and nothing was changed. Please try again.';
}

export function busyMessage(
  detail: { startedBy?: unknown; startedAt?: unknown; source?: unknown } | undefined,
  now: number,
): string {
  if (detail?.source === 'command_line') {
    return 'A repair is running from the command line. It has to finish first.';
  }
  if (typeof detail?.startedBy === 'string' && detail.startedBy && typeof detail.startedAt === 'number' && detail.startedAt > 0) {
    return `${detail.startedBy} started a repair ${relativeTime(detail.startedAt, now)}. It has to finish first.`;
  }
  return 'A repair is already running. It has to finish first.';
}
