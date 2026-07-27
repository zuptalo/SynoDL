import { describe, expect, it } from 'vitest';
import { reasonFor } from './task-error';

describe('reasonFor', () => {
  const known: Array<[string, string]> = [
    ['broken_link', 'Broken link'],
    ['destination_not_exist', 'Destination folder no longer exists'],
    ['destination_denied', 'No permission for the destination folder'],
    ['disk_full', 'The disk is full'],
    ['quota_reached', 'Storage quota reached'],
    ['timeout', 'The connection timed out'],
    ['torrent_duplicate', 'Already added'],
    ['required_premium', 'Requires a premium account'],
    ['name_too_long', 'The file name is too long'],
    ['exceed_max_fs_size', "Exceeds the filesystem's maximum file size"],
    ['try_it_later', 'Try again later'],
    ['extract_failed_wrong_password', 'Extraction failed: wrong password'],
  ];

  it.each(known)('maps %s to friendly text', (code, text) => {
    expect(reasonFor(code)).toBe(text);
  });

  it('falls back to a generic message for an unknown keyword', () => {
    expect(reasonFor('some_new_dsm_code')).toBe('Download failed');
  });

  it('falls back to a generic message for an empty or missing keyword', () => {
    expect(reasonFor('')).toBe('Download failed');
    expect(reasonFor(undefined as unknown as string)).toBe('Download failed');
  });

  it('is case-insensitive to the DSM keyword', () => {
    expect(reasonFor('BROKEN_LINK')).toBe('Broken link');
  });
});
