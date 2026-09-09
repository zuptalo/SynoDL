import { describe, expect, it } from 'vitest';
import { originOf, splitByOrigin } from './task-origin';
import type { Task } from '@/types/task';

const task = (id: string, over: Partial<Task> = {}): Task =>
  ({ id, name: id, type: 'bt', status: 'downloading', size: 1, downloaded: 0, ...over }) as Task;

describe('originOf', () => {
  it('reads a catalog id as "sent from Discover"', () => {
    expect(originOf(task('a', { catalogId: '1:x' }))).toBe('discover');
  });

  it('treats anything without one as added another way', () => {
    // No guessing from a poster or a media type: the catalog id is the one field
    // that exists for no other reason.
    expect(originOf(task('a'))).toBe('direct');
    expect(originOf(task('a', { catalogId: '' }))).toBe('direct');
    expect(originOf(task('a', { posterUrl: 'http://x/y.jpg', mediaType: 'movie' }))).toBe('direct');
  });
});

describe('splitByOrigin', () => {
  it('separates the two', () => {
    const { discover, direct } = splitByOrigin([
      task('a', { catalogId: '1:x' }),
      task('b'),
      task('c', { catalogId: '1:y' }),
    ]);
    expect(discover.map((t) => t.id)).toEqual(['a', 'c']);
    expect(direct.map((t) => t.id)).toEqual(['b']);
  });

  it('keeps the order it was given, so the filter sheet stays in charge', () => {
    const { discover } = splitByOrigin([
      task('z', { catalogId: '1' }),
      task('a', { catalogId: '2' }),
      task('m', { catalogId: '3' }),
    ]);
    expect(discover.map((t) => t.id)).toEqual(['z', 'a', 'm']);
  });

  it('gives empty lists rather than undefined, so a caller can just check length', () => {
    const { discover, direct } = splitByOrigin([]);
    expect(discover).toEqual([]);
    expect(direct).toEqual([]);
  });
});
