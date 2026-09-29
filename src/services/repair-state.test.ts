import { describe, expect, it } from 'vitest';
import type { RepairCheck, RepairPlan, RepairRun, RepairSnapshot } from '@/services/api';
import {
  busyMessage,
  checkRows,
  confirmLines,
  errorMessage,
  expiryText,
  formatBytes,
  formatCount,
  leftAloneView,
  offers,
  progressPercent,
  progressText,
  relativeTime,
  stageLabel,
  stateLabel,
  unavailableText,
} from '@/services/repair-state';

const NOW = 1_790_000_000;

function check(over: Partial<RepairCheck> = {}): RepairCheck {
  return {
    tracks: 5795, songs: 4017, duplicates: 1774, moves: 3970, retags: 4017, covers: 886, playlists: 88,
    conflicts: 0, nameClashes: 52, matched: 1828, noMatch: 2186, notLookedUp: 3, toSingles: 2917,
    albumKnown: 886, orphanNfo: 3484, bytesReclaimed: 13_368_173_185, bytesNeeded: 548_758_400,
    freeBytes: 4_600_000_000_000,
    leftAlone: { total: 6, byReason: [{ reason: 'no video id', count: 4 }, { reason: 'an mp3 of that name already exists', count: 2 }],
      examples: [{ path: 'A/b.mp3', reason: 'no video id' }] },
    ...over,
  };
}

function plan(over: Partial<RepairPlan> = {}): RepairPlan {
  return { id: '20260929T071341Z-73c844', checkedAt: NOW - 3600, expiresAt: NOW + 5 * 3600, status: 'ready',
    canApply: true, canContinue: false, summary: { kind: 'check', ok: true, check: check() }, ...over };
}

function snap(over: Partial<RepairSnapshot> = {}): RepairSnapshot {
  return { available: true, reason: '', current: null, plan: null, undo: null, latest: null, history: [], ...over };
}

function run(over: Partial<RepairRun> = {}): RepairRun {
  return { id: 'a1b2c3d4e5f6', kind: 'check', state: 'running', startedAt: NOW - 60, startedBy: 'Anna', ...over };
}

describe('formatting', () => {
  it('formats counts with grouping', () => {
    expect(formatCount(4017)).toBe('4,017');
    expect(formatCount(0)).toBe('0');
    expect(formatCount(-5)).toBe('0');
    expect(formatCount(Number.NaN)).toBe('0');
  });

  it('formats bytes in decimal units', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(999)).toBe('999 B');
    expect(formatBytes(1500)).toBe('1.5 kB');
    expect(formatBytes(13_368_173_185)).toBe('13.4 GB');
    expect(formatBytes(4_600_000_000_000)).toBe('4.6 TB');
    expect(formatBytes(-1)).toBe('0 B');
  });
});

describe('progress', () => {
  it('names each stage in plain words', () => {
    expect(stageLabel('scan')).toBe('Reading the library');
    expect(stageLabel('lookup')).toBe('Looking songs up');
    expect(stageLabel('plan')).toBe('Working out what to change');
    expect(stageLabel('apply')).toBe('Applying the changes');
    expect(stageLabel('restore')).toBe('Putting things back');
  });

  it('computes a percentage only when a total is known, and clamps it', () => {
    expect(progressPercent({ phase: 'lookup', done: 1000, total: 4000 })).toBe(25);
    expect(progressPercent({ phase: 'lookup', done: 5000, total: 4000 })).toBe(100);
    expect(progressPercent({ phase: 'scan', done: 0, total: 0 })).toBeNull();
    expect(progressPercent(undefined)).toBeNull();
  });

  it('words the reading', () => {
    expect(progressText({ phase: 'lookup', done: 1275, total: 4017 })).toBe('Looking songs up: 1,275 of 4,017');
    expect(progressText({ phase: 'scan', done: 0, total: 0 })).toBe('Reading the library');
    expect(progressText(undefined)).toBe('Starting…');
  });
});

describe('time', () => {
  it('says how long a check stays good', () => {
    expect(expiryText(plan({ expiresAt: NOW + 5 * 3600 + 100 }), NOW)).toBe('Good for 5 more hours');
    expect(expiryText(plan({ expiresAt: NOW + 3600 + 5 }), NOW)).toBe('Good for 1 more hour');
    expect(expiryText(plan({ expiresAt: NOW + 42 * 60 + 5 }), NOW)).toBe('Good for 42 more minutes');
    expect(expiryText(plan({ expiresAt: NOW + 20 }), NOW)).toBe('Good for less than a minute');
    expect(expiryText(plan({ expiresAt: NOW }), NOW)).toBe('Out of date: run the check again');
    expect(expiryText(plan({ expiresAt: NOW - 100 }), NOW)).toBe('Out of date: run the check again');
  });

  it('gives relative times', () => {
    expect(relativeTime(NOW - 10, NOW)).toBe('just now');
    expect(relativeTime(NOW - 5 * 60, NOW)).toBe('5 min ago');
    expect(relativeTime(NOW - 3 * 3600, NOW)).toBe('3 h ago');
    expect(relativeTime(NOW - 30 * 3600, NOW)).toBe('yesterday');
    expect(relativeTime(NOW - 5 * 86400, NOW)).toMatch(/\d{4}|[A-Z][a-z]{2}/);
    expect(relativeTime(NOW + 100, NOW)).toBe('just now');
  });
});

describe('the result', () => {
  it('lists the numbers an admin needs, in reading order', () => {
    const rows = checkRows(check());
    const by = Object.fromEntries(rows.map((r) => [r.key, r.value]));
    expect(by.duplicates).toBe('1,774');
    expect(by.moves).toBe('3,970');
    expect(by.retags).toBe('4,017');
    expect(by.playlists).toBe('88');
    expect(by.conflicts).toBe('0');
    expect(by.matched).toBe('1,828');
    expect(by.noMatch).toBe('2,186');
    expect(by.notLookedUp).toBe('3');
    expect(by.toSingles).toBe('2,917');
    expect(by.reclaimed).toBe('13.4 GB');
    expect(rows.map((r) => r.key).indexOf('duplicates')).toBeLessThan(rows.map((r) => r.key).indexOf('matched'));
  });

  it('compares the space needed with the space free', () => {
    const ok = checkRows(check()).find((r) => r.key === 'space')!;
    expect(ok.value).toBe('548.8 MB needed, 4.6 TB free');
    expect(ok.warn).toBeFalsy();
    const tight = checkRows(check({ bytesNeeded: 5_000_000_000_000, freeBytes: 4_600_000_000_000 })).find((r) => r.key === 'space')!;
    expect(tight.warn).toBe(true);
    // the tool refuses at 5% over, so the screen warns at the same line
    const edge = checkRows(check({ bytesNeeded: 1000, freeBytes: 1040 })).find((r) => r.key === 'space')!;
    expect(edge.warn).toBe(true);
    const unknown = checkRows(check({ freeBytes: 0 })).find((r) => r.key === 'space')!;
    expect(unknown.value).toBe('548.8 MB needed');
  });

  it('describes what was left alone as counts plus a few examples, and how many are not shown', () => {
    const v = leftAloneView(check().leftAlone);
    expect(v.total).toBe(6);
    expect(v.groups).toEqual([
      { reason: 'no video id', count: 4 },
      { reason: 'an mp3 of that name already exists', count: 2 },
    ]);
    expect(v.examples).toHaveLength(1);
    expect(v.hidden).toBe(5);
    expect(leftAloneView({ total: 0, byReason: [], examples: [] })).toEqual({ total: 0, groups: [], examples: [], hidden: 0 });
    expect(leftAloneView({ total: 1, byReason: [], examples: [{ path: 'a', reason: 'x' }] }).hidden).toBe(0);
  });
});

describe('the confirmation', () => {
  it('says what will happen, that nothing is deleted, and that it can be undone', () => {
    const lines = confirmLines(check());
    const text = lines.join('\n');
    expect(text).toContain('1,774');
    expect(text).toMatch(/\.trash/);
    expect(text).toMatch(/nothing is deleted/i);
    expect(text).toMatch(/undo/i);
    expect(text).toContain('3,970');
    expect(text).toContain('88');
  });

  it('drops a line whose number is zero rather than announcing "0 things"', () => {
    const text = confirmLines(check({ duplicates: 0, playlists: 0 })).join('\n');
    expect(text).not.toMatch(/\b0 duplicate/);
    expect(text).not.toMatch(/\b0 playlist/);
  });
});

describe('what the screen offers', () => {
  it('offers nothing but an explanation when unavailable', () => {
    const o = offers(snap({ available: false, reason: 'not_configured' }), NOW);
    expect(o).toMatchObject({ mode: 'unavailable', canCheck: false, canApply: false, canContinue: false, canUndo: false });
    expect(unavailableText('not_configured')).toMatch(/cluster|music library/i);
    expect(unavailableText('no_image')).toMatch(/image/i);
    expect(unavailableText('something else')).toMatch(/not available/i);
  });

  it('offers a check when idle, and shows a plan that can be applied', () => {
    const o = offers(snap({ plan: plan() }), NOW);
    expect(o).toMatchObject({ mode: 'idle', canCheck: true, canApply: true, showPlan: true });
  });

  it('offers nothing while a run is in progress', () => {
    const o = offers(snap({ current: run(), plan: plan(), undo: { planId: 'x', appliedAt: 1, canUndo: false } }), NOW);
    expect(o).toMatchObject({ mode: 'running', canCheck: false, canApply: false, canContinue: false, canUndo: false });
  });

  it('follows the server for apply, continue and undo, never inventing them', () => {
    expect(offers(snap({ plan: plan({ canApply: false, status: 'expired' }) }), NOW).canApply).toBe(false);
    expect(offers(snap({ plan: plan({ canApply: false, canContinue: true, status: 'apply_unfinished' }) }), NOW).canContinue).toBe(true);
    expect(offers(snap({ undo: { planId: 'x', appliedAt: 1, canUndo: true } }), NOW).canUndo).toBe(true);
    expect(offers(snap({ undo: { planId: 'x', appliedAt: 1, canUndo: false } }), NOW).canUndo).toBe(false);
  });

  it('still allows a new check when the last plan expired', () => {
    expect(offers(snap({ plan: plan({ status: 'expired', canApply: false }) }), NOW).canCheck).toBe(true);
  });
});

describe('labels', () => {
  it('names a run state in words', () => {
    expect(stateLabel('running')).toBe('Running');
    expect(stateLabel('finished')).toBe('Finished');
    expect(stateLabel('refused')).toBe('Refused');
    expect(stateLabel('unfinished')).toBe('Did not finish');
  });
});

describe('errors', () => {
  it('turns every server error code into a plain sentence', () => {
    for (const code of ['busy', 'no_plan', 'plan_expired', 'already_applied', 'not_undoable', 'snapshot_required', 'unavailable', 'start_failed', 'permission', 'bad_request']) {
      expect(errorMessage(code).length).toBeGreaterThan(10);
    }
    expect(errorMessage('something_new')).toMatch(/went wrong|could not/i);
  });

  it('says who is running and when, and that a command-line run is a command-line run', () => {
    expect(busyMessage({ startedBy: 'Anna', startedAt: NOW - 300, source: 'settings' }, NOW)).toBe('Anna started a repair 5 min ago. It has to finish first.');
    expect(busyMessage({ source: 'command_line' }, NOW)).toMatch(/command line/i);
    expect(busyMessage({}, NOW)).toMatch(/already running/i);
    expect(busyMessage(undefined, NOW)).toMatch(/already running/i);
  });
});
