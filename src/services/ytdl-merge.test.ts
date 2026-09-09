import { describe, expect, it } from 'vitest';
import { mergeYtdlUpdate, updateTouchesGroup } from './ytdl-merge';
import type { YtdlDownload } from './api';

function dl(requestId: string, over: Partial<YtdlDownload> = {}): YtdlDownload {
  return {
    requestId,
    kind: 'single',
    url: `https://youtu.be/${requestId}`,
    mode: 'music',
    scope: 'single',
    state: 'queued',
    ...over,
  } as YtdlDownload;
}

describe('mergeYtdlUpdate', () => {
  it('replaces a row it already holds', () => {
    const held = [dl('a'), dl('b')];
    const out = mergeYtdlUpdate(held, { changed: [dl('a', { state: 'downloading', progress: 0.4 })] });
    expect(out.map((d) => d.requestId)).toEqual(['a', 'b']);
    expect(out[0].state).toBe('downloading');
    expect(out[0].progress).toBe(0.4);
    expect(out[1]).toBe(held[1]); // untouched rows keep their identity
  });

  it('ignores a change about a row it has never loaded', () => {
    // FR-004a. History is unbounded and the list is paged, so an update is not
    // an instruction to go and fetch a page.
    const held = [dl('a')];
    const out = mergeYtdlUpdate(held, { changed: [dl('zzz', { state: 'completed' })] });
    expect(out.map((d) => d.requestId)).toEqual(['a']);
  });

  it('inserts a newly created top-level download at the top', () => {
    // Without this an admin watching would never see somebody else's
    // submission appear, because nothing polls any more. The list is
    // newest-first, so the top is where a new row belongs.
    const out = mergeYtdlUpdate([dl('a')], { created: [dl('new')] });
    expect(out.map((d) => d.requestId)).toEqual(['new', 'a']);
  });

  it('does not insert a created row that belongs behind a group', () => {
    // An item reaches a client only while that group's sheet is open, and then
    // it is already held. An unheld one belongs to a group nobody is looking at.
    const out = mergeYtdlUpdate([dl('a')], {
      created: [dl('item', { parentId: 'grp', kind: 'item' })],
    });
    expect(out.map((d) => d.requestId)).toEqual(['a']);
  });

  it('never inserts into a group sheet, only merges', () => {
    const held = [dl('i1', { parentId: 'grp' })];
    const out = mergeYtdlUpdate(
      held,
      { created: [dl('i2', { parentId: 'grp' })], changed: [dl('i1', { parentId: 'grp', state: 'completed' })] },
      true,
    );
    expect(out.map((d) => d.requestId)).toEqual(['i1']);
    expect(out[0].state).toBe('completed');
  });

  it('drops a removed row', () => {
    const out = mergeYtdlUpdate([dl('a'), dl('b')], { removed: ['a'] });
    expect(out.map((d) => d.requestId)).toEqual(['b']);
  });

  it('merges rather than duplicates a created row it already holds', () => {
    // A retried download re-enters the server's unfinished set and looks new to
    // it, because it had been forgotten. The client still has it.
    const out = mergeYtdlUpdate([dl('a', { state: 'failed' })], {
      created: [dl('a', { state: 'queued' })],
    });
    expect(out).toHaveLength(1);
    expect(out[0].state).toBe('queued');
  });

  it('lets a removal win over a change in the same update', () => {
    const out = mergeYtdlUpdate([dl('a')], {
      changed: [dl('a', { state: 'downloading' })],
      removed: ['a'],
    });
    expect(out).toEqual([]);
  });

  it('leaves the list alone for an empty update', () => {
    const held = [dl('a')];
    expect(mergeYtdlUpdate(held, {})).toEqual(held);
  });
});

describe('updateTouchesGroup', () => {
  it('is true for one of the group items', () => {
    expect(updateTouchesGroup({ changed: [dl('i', { parentId: 'grp' })] }, 'grp')).toBe(true);
  });

  it('is true for the group row itself, whose counts move as items finish', () => {
    expect(updateTouchesGroup({ changed: [dl('grp', { kind: 'group' })] }, 'grp')).toBe(true);
  });

  it('is false for an unrelated download', () => {
    // A group of several hundred tracks must not re-render because something
    // else entirely moved.
    expect(updateTouchesGroup({ changed: [dl('other')] }, 'grp')).toBe(false);
  });

  it('is true for any removal, since a removal carries no parent', () => {
    expect(updateTouchesGroup({ removed: ['i'] }, 'grp')).toBe(true);
  });
});
