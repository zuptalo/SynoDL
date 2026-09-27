import type { YtdlState } from '@/services/api';

/**
 * What each YouTube download state is called on screen (spec 1049).
 *
 * One table for the row chip and the detail sheet, so the two cannot drift
 * apart — they did, as two hand-written copies. Typed against the state union
 * so a new state fails the typecheck here instead of rendering an empty chip,
 * which is exactly what "nothing for when in the queue" looked like.
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
