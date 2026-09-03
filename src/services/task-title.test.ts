import { describe, expect, it } from 'vitest';
import { taskTitle } from './task-title';

const base = { name: 'file.mkv', destination: '', uri: undefined };

describe('taskTitle', () => {
  it('uses the destination folder as the title, with the year split off', () => {
    const t = taskTitle({ ...base, destination: 'movies/Despicable Me 4 2024' });
    expect(t).toEqual({ title: 'Despicable Me 4', episode: '', year: '2024' });
  });

  it('strips a series year-range from the folder title', () => {
    const t = taskTitle({
      name: 'Dexter.Resurrection.S01E10.mkv',
      destination: 'series/Dexter Resurrection 2025',
      uri: undefined,
    });
    expect(t).toEqual({ title: 'Dexter Resurrection', episode: 'S01E10', year: '2025' });
  });

  it('extracts season/episode from the file name for a series', () => {
    const t = taskTitle({
      name: 'Rick.and.Morty.S01E05.1080p.WEB-DL.mkv',
      destination: 'tv-show/Rick and Morty',
      uri: undefined,
    });
    expect(t).toEqual({ title: 'Rick and Morty', episode: 'S01E05', year: '' });
  });

  it('extracts season/episode from underscore-separated names (the 30nama format)', () => {
    expect(
      taskTitle({
        name: 'X_Men_97_S01E01_1080p_WEB-DL_TheCuteness_30NAMA.mkv',
        destination: 'series/x_men_97',
        uri: undefined,
      }).episode,
    ).toBe('S01E01');
    expect(
      taskTitle({
        name: 'The_Big_Bang_Theory_S02E01_10bit_x265_1080p_BluRay_RCVR_30NAMA.mkv',
        destination: 'series/the_big_bang_theory',
        uri: undefined,
      }).episode,
    ).toBe('S02E01');
  });

  it('reads the episode from the download URL path (underscore-separated)', () => {
    expect(
      taskTitle({
        name: 'opaque',
        destination: 'series/x_men_97',
        uri: 'https://host/download/.../series/x_men_97/X_Men_97_S01E10_1080p_WEB-DL_30NAMA.mkv',
      }).episode,
    ).toBe('S01E10');
  });

  it('pads and reads the episode from the link when the name lacks it', () => {
    const t = taskTitle({
      name: 'ep.mkv',
      destination: 'tv/Show',
      uri: 'https://cdn/x/s2e7/file.mkv',
    });
    expect(t).toEqual({ title: 'Show', episode: 'S02E07', year: '' });
  });

  it('falls back to the raw name when there is no folder', () => {
    const t = taskTitle({ name: 'linux.iso', destination: '', uri: undefined });
    expect(t).toEqual({ title: 'linux.iso', episode: '', year: '' });
  });

  it('keeps the raw name for a non-media (generic) folder', () => {
    const t = taskTitle({ name: 'e2e-fixture.iso', destination: 'home/Downloads', uri: undefined });
    expect(t).toEqual({ title: 'e2e-fixture.iso', episode: '', year: '' });
  });
});

// Spec 1021: a series now lands in "Show (Year)/Season NN", so the leaf of the
// destination is the SEASON, not the title. Without stepping up, every episode
// in the Tasks list would read "Season 01".
describe('season subfolders (spec 1021)', () => {
  it('takes the title from the show folder, not the season folder', () => {
    expect(
      taskTitle({
        name: 'Friends.S01E05.1080p.WEB-DL.mkv',
        destination: 'tv-show/Friends (1994)/Season 01',
        uri: '',
      }),
    ).toEqual({ title: 'Friends', episode: 'S01E05', year: '1994' });
  });

  it('handles a season folder without a year on the show', () => {
    expect(
      taskTitle({
        name: 'Whiskey.S01E02.mkv',
        destination: 'tv-show/Whiskey on the Rocks/Season 01',
        uri: '',
      }),
    ).toEqual({ title: 'Whiskey on the Rocks', episode: 'S01E02', year: '' });
  });

  it('still reads a movie in the new naming', () => {
    expect(
      taskTitle({
        name: 'Despicable.Me.4.2024.2160p.mkv',
        destination: 'movie/Despicable Me 4 (2024)',
        uri: '',
      }),
    ).toEqual({ title: 'Despicable Me 4', episode: '', year: '2024' });
  });

  // FR-011: downloads sent before the change keep working.
  it('still reads the old flat naming', () => {
    expect(
      taskTitle({
        name: 'Friends.S02E01.mkv',
        destination: 'tv-show/Friends 1994 - 2004',
        uri: '',
      }),
    ).toEqual({ title: 'Friends', episode: 'S02E01', year: '1994 – 2004' });
  });

  // A folder that merely starts with "Season" is not a season folder.
  it('does not mistake a title for a season folder', () => {
    expect(
      taskTitle({
        name: 'Seasons.of.Love.2019.mkv',
        destination: 'movie/Seasons of Love (2019)',
        uri: '',
      }).title,
    ).toBe('Seasons of Love');
  });
});
