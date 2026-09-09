import { describe, expect, it } from 'vitest';
import { applyYtdlFilter } from './ytdl-sort';
import { defaultTaskFilter, type TaskFilterState } from './task-sort';
import type { YtdlDownload } from './api';

function dl(requestId: string, over: Partial<YtdlDownload> = {}): YtdlDownload {
  return {
    requestId,
    kind: 'single',
    url: `https://youtu.be/${requestId}`,
    mode: 'music',
    scope: 'single',
    state: 'queued',
    submittedAt: 1_000,
    ...over,
  } as YtdlDownload;
}

function filter(over: Partial<TaskFilterState> = {}): TaskFilterState {
  return { ...defaultTaskFilter(), ...over };
}

const ids = (ds: YtdlDownload[]) => ds.map((d) => d.requestId);

describe('applyYtdlFilter — searching', () => {
  const rows = [
    dl('a', { title: 'Anyma - Sonder', uploader: 'Anyma' }),
    dl('b', { title: 'Bohemian Rhapsody', uploader: 'Queen' }),
    dl('c', { title: 'Lucente', groupName: 'The End Of Genesys' }),
  ];

  it('matches the title the row actually shows', () => {
    // The reported bug: a row shows its title, and searching for that title
    // found nothing because only the URL was matched.
    expect(ids(applyYtdlFilter(rows, filter({ term: 'sonder' })))).toEqual(['a']);
  });

  it('matches the artist', () => {
    expect(ids(applyYtdlFilter(rows, filter({ term: 'queen' })))).toEqual(['b']);
  });

  it('matches the playlist an item came from', () => {
    expect(ids(applyYtdlFilter(rows, filter({ term: 'genesys' })))).toEqual(['c']);
  });

  it('still matches the link, for a download that has no title yet', () => {
    expect(ids(applyYtdlFilter(rows, filter({ term: 'youtu.be/b' })))).toEqual(['b']);
  });

  it('keeps everything when nothing is typed', () => {
    expect(applyYtdlFilter(rows, filter())).toHaveLength(3);
  });
});

describe('applyYtdlFilter — sorting', () => {
  it('sorts by name, using the title a row shows', () => {
    const rows = [dl('a', { title: 'Zulu' }), dl('b', { title: 'Alpha' })];
    expect(ids(applyYtdlFilter(rows, filter({ sortKey: 'name', ascending: true })))).toEqual([
      'b',
      'a',
    ]);
    expect(ids(applyYtdlFilter(rows, filter({ sortKey: 'name', ascending: false })))).toEqual([
      'a',
      'b',
    ]);
  });

  it('sorts by state with the ones doing something first', () => {
    const rows = [
      dl('done', { state: 'completed' }),
      dl('run', { state: 'downloading' }),
      dl('wait', { state: 'queued' }),
    ];
    expect(ids(applyYtdlFilter(rows, filter({ sortKey: 'status', ascending: true })))).toEqual([
      'run',
      'wait',
      'done',
    ]);
  });

  it('sorts by progress, counting a finished download as complete', () => {
    const rows = [
      dl('half', { state: 'downloading', progress: 0.5 }),
      dl('done', { state: 'completed' }),
      dl('none', { state: 'queued' }),
    ];
    expect(ids(applyYtdlFilter(rows, filter({ sortKey: 'progress', ascending: true })))).toEqual([
      'none',
      'half',
      'done',
    ]);
  });

  it('sorts a group by how many of its items are saved', () => {
    const rows = [
      dl('few', { kind: 'group', counts: { total: 10, completed: 2, failed: 0, remaining: 8 } }),
      dl('most', { kind: 'group', counts: { total: 10, completed: 9, failed: 0, remaining: 1 } }),
    ];
    expect(ids(applyYtdlFilter(rows, filter({ sortKey: 'progress', ascending: true })))).toEqual([
      'few',
      'most',
    ]);
  });

  it('sorts newest first by default', () => {
    const rows = [dl('old', { submittedAt: 100 }), dl('new', { submittedAt: 200 })];
    expect(ids(applyYtdlFilter(rows, filter()))).toEqual(['new', 'old']);
  });

  it('leaves the section in its default order for a sort only a NAS task has', () => {
    // Peers, ratio, speeds: a worker on a cluster has none of them. Inventing a
    // zero would scramble the section into whatever the tie-break produced;
    // falling back to when it was asked for leaves it looking untouched.
    const rows = [dl('old', { submittedAt: 100 }), dl('new', { submittedAt: 200 })];
    for (const sortKey of ['peers', 'ratio', 'uploadSpeed', 'size', 'remaining'] as const) {
      expect(ids(applyYtdlFilter(rows, filter({ sortKey })))).toEqual(['new', 'old']);
    }
  });

  it('does not mutate what it was given', () => {
    const rows = [dl('a', { title: 'Zulu' }), dl('b', { title: 'Alpha' })];
    applyYtdlFilter(rows, filter({ sortKey: 'name', ascending: true }));
    expect(ids(rows)).toEqual(['a', 'b']);
  });

  it('breaks a tie the same way in both directions, so the order is stable', () => {
    const rows = [dl('a', { title: 'Same' }), dl('b', { title: 'Same' })];
    const asc = ids(applyYtdlFilter(rows, filter({ sortKey: 'name', ascending: true })));
    const desc = ids(applyYtdlFilter(rows, filter({ sortKey: 'name', ascending: false })));
    expect(asc).toEqual(desc);
  });
});

describe('applyYtdlFilter — the cases with nothing to go on', () => {
  it('falls back to the link when a download has no title yet', () => {
    // A channel publishes no metadata document, so its row shows its link and
    // sorting by name has to use the same thing the reader can see.
    const rows = [dl('zzz', { url: 'https://youtu.be/zzz' }), dl('aaa', { url: 'https://youtu.be/aaa' })];
    expect(ids(applyYtdlFilter(rows, filter({ sortKey: 'name', ascending: true })))).toEqual([
      'aaa',
      'zzz',
    ]);
  });

  it('places a state it does not recognise among the waiting ones, not off the end', () => {
    const rows = [
      dl('run', { state: 'downloading' }),
      dl('odd', { state: 'something-new' as YtdlDownload['state'] }),
      dl('done', { state: 'completed' }),
    ];
    expect(ids(applyYtdlFilter(rows, filter({ sortKey: 'status', ascending: true })))).toEqual([
      'run',
      'odd',
      'done',
    ]);
  });

  it('treats a running download with no reading as least far along', () => {
    // The row correctly shows no bar when nothing is known; in an ordering it is
    // still the least-far-along thing that is running.
    const rows = [
      dl('known', { state: 'downloading', progress: 0.3 }),
      dl('unknown', { state: 'downloading' }),
    ];
    expect(ids(applyYtdlFilter(rows, filter({ sortKey: 'progress', ascending: true })))).toEqual([
      'unknown',
      'known',
    ]);
  });

  it('orders rows with no submission time at all by id, so the list never jitters', () => {
    const rows = [
      dl('b', { submittedAt: undefined }),
      dl('a', { submittedAt: undefined }),
    ];
    // Descending by id, the same direction the NAS list's "newest first"
    // tie-break runs in. What matters is that it is the SAME every time.
    const once = ids(applyYtdlFilter(rows, filter()));
    const twice = ids(applyYtdlFilter(rows, filter()));
    expect(once).toEqual(twice);
    expect(once).toEqual(['b', 'a']);
  });

  it('copes with a filter carrying no term at all', () => {
    const rows = [dl('a')];
    expect(applyYtdlFilter(rows, { ...filter(), term: undefined as unknown as string })).toHaveLength(1);
  });
});
