/**
 * Which block of the Tasks list goes on top.
 *
 * The three kinds — uploads, YouTube downloads and NAS downloads — are separate
 * systems with separate lists, and their order used to be hard-coded. That put
 * whatever you had just started underneath whatever you did last week: send a
 * film from Discover while a YouTube playlist is saving and the new thing is
 * below twelve finished tracks.
 *
 * So the block holding the most recently added thing comes first. Ordering by
 * the NEWEST member rather than by any kind of average is deliberate: the
 * question a reader is asking when they open this page is "what about the thing
 * I just did", and only the newest member answers it.
 *
 * Timestamps are unix SECONDS throughout, which is what both the NAS
 * (`Task.createdAt`) and the server (`submittedAt`, a `time.Now().Unix()`)
 * already speak. Uploads are stamped client-side in the same unit rather than
 * in milliseconds, so the three are comparable without anyone remembering to
 * convert.
 */
export type SectionKey = 'uploads' | 'ytdl' | 'downloads';

/**
 * The order used when nothing distinguishes two blocks — an empty list, or a
 * genuine tie. Uploads first because they are the only one that needs the app
 * kept open, so they are the one worth seeing.
 */
const DEFAULT_ORDER: readonly SectionKey[] = ['uploads', 'ytdl', 'downloads'];

/**
 * Order the blocks by the newest thing in each.
 *
 * A block with no timestamp — because it is empty, or because nothing in it
 * carries one — sorts last rather than first. An absent time is not a recent
 * one, and treating it as 0 would be the same answer for the wrong reason.
 */
export function orderSections(newest: Partial<Record<SectionKey, number>>): SectionKey[] {
  return [...DEFAULT_ORDER].sort((a, b) => {
    const ta = newest[a];
    const tb = newest[b];
    if (ta === tb) return DEFAULT_ORDER.indexOf(a) - DEFAULT_ORDER.indexOf(b);
    if (ta === undefined) return 1;
    if (tb === undefined) return -1;
    if (tb !== ta) return tb - ta;
    return DEFAULT_ORDER.indexOf(a) - DEFAULT_ORDER.indexOf(b);
  });
}

/** The newest timestamp in a list, or undefined when there is nothing to take. */
export function newestOf<T>(items: readonly T[], at: (item: T) => number | undefined): number | undefined {
  let best: number | undefined;
  for (const item of items) {
    const t = at(item);
    if (t === undefined || !Number.isFinite(t) || t <= 0) continue;
    if (best === undefined || t > best) best = t;
  }
  return best;
}
