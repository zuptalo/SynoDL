/**
 * A readable title for a task row. Downloads sent from Discover are placed in a
 * destination folder named after the title (e.g. `movies/Despicable Me 4 2024`
 * or `tv-show/Rick and Morty`), which is a far cleaner label than the raw file
 * name Download Station reports. For series, the season/episode is pulled from
 * the file name or the source link (`…S01E05…`) when present.
 */
import type { Task } from '@/types/task';

// S01E05 / s1.e5 / S01 E05 … anywhere in a file name or URL. We bound on
// NON-alphanumerics (or the string edges) rather than \b, because the source
// separates with underscores — "X_Men_97_S01E01_1080p_…" — and \b treats "_" as
// a word char, so it would never fire between "_" and "S" (the original bug).
const SE_RE = /(?:^|[^a-z0-9])s(\d{1,2})[^a-z0-9]?e(\d{1,3})(?=$|[^a-z0-9])/i;

function episodeOf(...sources: (string | undefined)[]): string {
  for (const s of sources) {
    if (!s) continue;
    const m = SE_RE.exec(s);
    if (m) return `S${m[1].padStart(2, '0')}E${m[2].padStart(2, '0')}`;
  }
  return '';
}

// Parent folders that mark a media download (the source send nests each title
// under a movies/tv parent). Used to decide when the folder is a real title vs a
// generic download folder where the file name is more meaningful.
const MEDIA_PARENT = /^(movie|movies|tv|tv-?shows?|series|anime|video|videos)$/i;

function pathParts(destination: string | undefined): string[] {
  return (destination ?? '').split('/').filter(Boolean);
}
// The leaf of the destination path — the per-title folder we created.
function folderTitle(destination: string | undefined): string {
  const parts = pathParts(destination);
  return parts.length ? parts[parts.length - 1] : '';
}
function isMediaFolder(destination: string | undefined): boolean {
  const parts = pathParts(destination);
  return parts.length >= 2 && MEDIA_PARENT.test(parts[parts.length - 2]);
}

export interface TaskTitle {
  /** The human-readable title (folder name, or the raw task name as a fallback). */
  title: string;
  /** "S01E05" when this is a detectable series episode, otherwise "". */
  episode: string;
}

export function taskTitle(task: Pick<Task, 'name' | 'destination' | 'uri'>): TaskTitle {
  const episode = episodeOf(task.name, task.uri);
  const folder = folderTitle(task.destination);
  // Prefer the download's folder as the title only when it's clearly a media
  // download — a movies/tv parent, or a detected episode. Otherwise the raw name
  // is more meaningful (e.g. a file dropped in a generic "Downloads" folder).
  const useFolder = folder !== '' && (episode !== '' || isMediaFolder(task.destination));
  return { title: useFolder ? folder : task.name, episode };
}
