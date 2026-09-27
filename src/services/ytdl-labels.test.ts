import { describe, expect, it } from 'vitest';
import type { YtdlState } from './api';
import { YTDL_STATE_LABELS, ytdlStateLabel } from './ytdl-labels';

const STATES: YtdlState[] = ['resolving', 'queued', 'scheduled', 'downloading', 'completed', 'failed'];

describe('ytdlStateLabel', () => {
  it('names the four states the way the NAS list does', () => {
    expect(ytdlStateLabel('completed')).toBe('Finished');
    expect(ytdlStateLabel('failed')).toBe('Failed');
    expect(ytdlStateLabel('queued')).toBe('Pending');
    expect(ytdlStateLabel('downloading')).toBe('Downloading');
  });

  it('starts every label with a capital letter and leaves none empty', () => {
    for (const s of STATES) {
      const label = YTDL_STATE_LABELS[s];
      expect(label.length).toBeGreaterThan(0);
      expect(label[0]).toBe(label[0].toUpperCase());
      expect(label[0]).not.toBe(label[0].toLowerCase());
    }
  });
});
