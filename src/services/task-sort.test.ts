import { describe, expect, it } from 'vitest';
import type { Task } from '@/types/task';
import {
  ALL_STATUSES,
  applyTaskFilter,
  defaultTaskFilter,
  type TaskFilterState,
} from './task-sort';

function task(overrides: Partial<Task> & { id: string }): Task {
  return {
    name: overrides.id,
    type: 'bt',
    status: 'downloading',
    size: 1000,
    downloaded: 500,
    uploaded: 0,
    downloadSpeed: 100,
    uploadSpeed: 0,
    peers: 0,
    seeders: 0,
    createdAt: 0,
    destination: '',
    ...overrides,
  };
}

function f(overrides: Partial<TaskFilterState>): TaskFilterState {
  return { ...defaultTaskFilter(), ...overrides };
}

const ids = (tasks: Task[]) => tasks.map((t) => t.id);

describe('applyTaskFilter — sorting', () => {
  const mixed = [
    task({ id: 'a', size: 300, createdAt: 30, peers: 1, downloadSpeed: 5, uploadSpeed: 9, name: 'Charlie', downloaded: 300, uploaded: 600 }),
    task({ id: 'b', size: 100, createdAt: 10, peers: 3, downloadSpeed: 20, uploadSpeed: 1, name: 'alpha', downloaded: 50, uploaded: 25 }),
    task({ id: 'c', size: 200, createdAt: 20, peers: 2, downloadSpeed: 10, uploadSpeed: 5, name: 'bravo', downloaded: 100, uploaded: 300 }),
  ];

  it('sorts by size both directions', () => {
    expect(ids(applyTaskFilter(mixed, f({ sortKey: 'size', ascending: true })))).toEqual(['b', 'c', 'a']);
    expect(ids(applyTaskFilter(mixed, f({ sortKey: 'size', ascending: false })))).toEqual(['a', 'c', 'b']);
  });

  it('sorts by creation date (default: newest first)', () => {
    expect(ids(applyTaskFilter(mixed, defaultTaskFilter()))).toEqual(['a', 'c', 'b']);
    expect(ids(applyTaskFilter(mixed, f({ ascending: true })))).toEqual(['b', 'c', 'a']);
  });

  it('sorts by peers, download speed, upload speed', () => {
    expect(ids(applyTaskFilter(mixed, f({ sortKey: 'peers', ascending: false })))).toEqual(['b', 'c', 'a']);
    expect(ids(applyTaskFilter(mixed, f({ sortKey: 'downloadSpeed', ascending: false })))).toEqual(['b', 'c', 'a']);
    expect(ids(applyTaskFilter(mixed, f({ sortKey: 'uploadSpeed', ascending: false })))).toEqual(['a', 'c', 'b']);
  });

  it('sorts by name case-insensitively', () => {
    expect(ids(applyTaskFilter(mixed, f({ sortKey: 'name', ascending: true })))).toEqual(['b', 'c', 'a']);
  });

  it('sorts by share ratio (uploaded/downloaded; zero downloaded = ratio 0)', () => {
    const withZero = [...mixed, task({ id: 'z', downloaded: 0, uploaded: 999 })];
    // ratios: a=2, b=0.5, c=3, z=0
    expect(ids(applyTaskFilter(withZero, f({ sortKey: 'ratio', ascending: false })))).toEqual(['c', 'a', 'b', 'z']);
  });

  it('sorts by progress (size 0 counts as 0 progress)', () => {
    const withZero = [...mixed, task({ id: 'z', size: 0, downloaded: 0 })];
    // progress: a=1, b=0.5, c=0.5 (b,c tie → id order), z=0
    expect(ids(applyTaskFilter(withZero, f({ sortKey: 'progress', ascending: false })))).toEqual(['a', 'b', 'c', 'z']);
  });

  it('sorts by remaining time with unknown (no speed / no size) always last', () => {
    const tasks = [
      task({ id: 'slow', size: 1000, downloaded: 0, downloadSpeed: 1 }), // 1000s
      task({ id: 'fast', size: 1000, downloaded: 900, downloadSpeed: 100 }), // 1s
      task({ id: 'stalled', size: 1000, downloaded: 0, downloadSpeed: 0 }), // unknown
      task({ id: 'meta', size: 0, downloaded: 0, downloadSpeed: 50 }), // unknown
    ];
    // Unknowns settle among themselves by id (deterministic, input-order-free).
    expect(ids(applyTaskFilter(tasks, f({ sortKey: 'remaining', ascending: true })))).toEqual(['fast', 'slow', 'meta', 'stalled']);
    // Unknown stays last even descending — it's "no estimate", not "longest".
    expect(ids(applyTaskFilter(tasks, f({ sortKey: 'remaining', ascending: false })))).toEqual(['slow', 'fast', 'meta', 'stalled']);
  });

  it('breaks ties stably by id', () => {
    const tied = [task({ id: 'x', size: 5 }), task({ id: 'y', size: 5 }), task({ id: 'w', size: 5 })];
    expect(ids(applyTaskFilter(tied, f({ sortKey: 'size', ascending: true })))).toEqual(['w', 'x', 'y']);
  });

  it('sorts by status in lifecycle order', () => {
    const tasks = [
      task({ id: 'fin', status: 'finished' }),
      task({ id: 'dl', status: 'downloading' }),
      task({ id: 'err', status: 'error' }),
      task({ id: 'seed', status: 'seeding' }),
    ];
    expect(ids(applyTaskFilter(tasks, f({ sortKey: 'status', ascending: true })))).toEqual(['dl', 'seed', 'fin', 'err']);
  });
});

describe('applyTaskFilter — term + status filters', () => {
  const tasks = [
    task({ id: 'u', name: 'Ubuntu ISO', status: 'downloading' }),
    task({ id: 'd', name: 'debian netinst', status: 'finished' }),
    task({ id: 'm', name: 'movie.mkv', status: 'paused' }),
  ];

  it('term filter is case-insensitive substring on the name', () => {
    expect(ids(applyTaskFilter(tasks, f({ term: 'UBUNTU' })))).toEqual(['u']);
    expect(ids(applyTaskFilter(tasks, f({ term: 'net' })))).toEqual(['d']);
    expect(ids(applyTaskFilter(tasks, f({ term: '' })))).toHaveLength(3);
  });

  it('status multi-select keeps only enabled statuses', () => {
    // All createdAt tie → id order ('m' < 'u') under the default sort.
    expect(ids(applyTaskFilter(tasks, f({ statuses: ['downloading', 'paused'] })))).toEqual(['m', 'u']);
    expect(ids(applyTaskFilter(tasks, f({ statuses: [] })))).toEqual([]);
  });

  it('an unknown/new DSM status is never silently hidden by the default filter', () => {
    const exotic = [task({ id: 'x', status: 'repairing' })];
    expect(ids(applyTaskFilter(exotic, defaultTaskFilter()))).toEqual(['x']);
  });
});

describe('defaults', () => {
  it('default filter: createdAt desc, empty term, all twelve statuses on', () => {
    const d = defaultTaskFilter();
    expect(d.sortKey).toBe('createdAt');
    expect(d.ascending).toBe(false);
    expect(d.term).toBe('');
    expect(d.statuses).toEqual(ALL_STATUSES);
    expect(ALL_STATUSES).toHaveLength(12);
  });
});
