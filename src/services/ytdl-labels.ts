import type { YtdlState } from '@/services/api';

/**
 * What each YouTube download state is called on screen (spec 1049).
 *
 * One table for the row chip and the detail sheet, so the two cannot drift
 * apart — they were two hand-written copies. Typed against the state union so
 * a state added later fails the typecheck here instead of rendering an empty
 * chip.
 *
 * "Pending" replaces "waiting its turn", which did not read as a state at all
 * next to the others — a queued download looked as if it had none.
 *
 * The words match the NAS task chips ("Finished", "Downloading", "Waiting"),
 * because the two lists sit on one screen and read as one: a YouTube track and
 * a NAS torrent that are both done should not be "saved" and "Finished". Every
 * label starts with a capital letter for the same reason.
 */
export const YTDL_STATE_LABELS: Record<YtdlState, string> = {
  resolving: 'Reading contents',
  queued: 'Pending',
  scheduled: 'Starting',
  downloading: 'Downloading',
  completed: 'Finished',
  failed: 'Failed',
};

export function ytdlStateLabel(state: YtdlState): string {
  return YTDL_STATE_LABELS[state];
}
