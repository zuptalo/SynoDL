/**
 * Maps a DSM task `status_extra.error_detail` keyword to a short, human-readable
 * reason for an errored download. DSM version quirks in the keyword set are
 * absorbed here so the UI stays dumb; unknown or missing keywords degrade to a
 * generic message rather than showing a raw code. Pure + unit-tested (on the
 * vitest coverage allowlist).
 */

const REASONS: Record<string, string> = {
  broken_link: 'Broken link',
  destination_not_exist: 'Destination folder no longer exists',
  destination_denied: 'No permission for the destination folder',
  disk_full: 'The disk is full',
  quota_reached: 'Storage quota reached',
  timeout: 'The connection timed out',
  exceed_max_fs_size: "Exceeds the filesystem's maximum file size",
  exceed_max_temp_size: 'Exceeds the temporary-folder size limit',
  exceed_max_dest_size: 'Exceeds the destination size limit',
  name_too_long: 'The file name is too long',
  torrent_duplicate: 'Already added',
  required_premium: 'Requires a premium account',
  try_it_later: 'Try again later',
  encryption: 'Encryption error',
  decryption: 'Encryption error',
  missing_python: 'Python is required on the NAS',
  private_video: 'The video is private',
  ftp_encryption_not_supported: 'The server requires unsupported FTP encryption',
  extract_failed: 'Extraction failed',
  extract_failed_disk_full: 'Extraction failed: the disk is full',
  extract_failed_invalid_archive: 'Extraction failed: invalid archive',
  extract_failed_quota_reached: 'Extraction failed: storage quota reached',
  extract_failed_wrong_password: 'Extraction failed: wrong password',
  unknown: 'Download failed',
};

const GENERIC = 'Download failed';

/** DSM error_detail keyword → friendly reason; generic fallback for unknown/empty. */
export function reasonFor(errorDetail: string): string {
  if (!errorDetail) return GENERIC;
  return REASONS[errorDetail.toLowerCase()] ?? GENERIC;
}
