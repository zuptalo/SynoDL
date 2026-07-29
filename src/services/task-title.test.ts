import { describe, expect, it } from 'vitest';
import { taskTitle } from './task-title';

const base = { name: 'file.mkv', destination: '', uri: undefined };

describe('taskTitle', () => {
  it('uses the destination folder as the title', () => {
    const t = taskTitle({ ...base, destination: 'movies/Despicable Me 4 2024' });
    expect(t).toEqual({ title: 'Despicable Me 4 2024', episode: '' });
  });

  it('extracts season/episode from the file name for a series', () => {
    const t = taskTitle({
      name: 'Rick.and.Morty.S01E05.1080p.WEB-DL.mkv',
      destination: 'tv-show/Rick and Morty',
      uri: undefined,
    });
    expect(t).toEqual({ title: 'Rick and Morty', episode: 'S01E05' });
  });

  it('pads and reads the episode from the link when the name lacks it', () => {
    const t = taskTitle({
      name: 'ep.mkv',
      destination: 'tv/Show',
      uri: 'https://cdn/x/s2e7/file.mkv',
    });
    expect(t).toEqual({ title: 'Show', episode: 'S02E07' });
  });

  it('falls back to the raw name when there is no folder', () => {
    const t = taskTitle({ name: 'linux.iso', destination: '', uri: undefined });
    expect(t).toEqual({ title: 'linux.iso', episode: '' });
  });

  it('keeps the raw name for a non-media (generic) folder', () => {
    const t = taskTitle({ name: 'e2e-fixture.iso', destination: 'home/Downloads', uri: undefined });
    expect(t).toEqual({ title: 'e2e-fixture.iso', episode: '' });
  });
});
