/**
 * Which published size of a YouTube thumbnail to ask for (spec 1039).
 *
 * The source publishes the same frame at several sizes, and they are not all the
 * same SHAPE. That is the whole reason this exists:
 *
 *   mqdefault    320×180    16:9, the real frame
 *   hqdefault    480×360     4:3, the frame with black bands above and below
 *   maxresdefault 1280×720   16:9, sharp, but not published for every video
 *
 * Artwork was stored as `hqdefault` — so the row's 40×60 poster slot cropped a
 * letterboxed image, keeping the bands and about a third of the width. That is
 * why the tiles read as dark slivers beside a film poster that fills its slot.
 *
 * The size is chosen HERE, where the image is rendered, rather than in what is
 * stored. Records therefore need no migration, every existing download benefits,
 * and changing this decision again costs nothing.
 */

/** The sizes this app asks for, smallest first. */
export type YtdlThumbSize = 'mq' | 'maxres';

const FILENAME: Record<YtdlThumbSize, string> = {
  mq: 'mqdefault.jpg',
  maxres: 'maxresdefault.jpg',
};

// A thumbnail path is /vi/<videoId>/<size>.jpg — sometimes under /vi_webp/. Only
// the last segment is rewritten; the id is left exactly as it was found.
const THUMB_PATH = /^\/(vi|vi_webp)\/([^/]+)\/[^/]+$/;

/**
 * Rewrite a stored artwork URL to ask for `size`.
 *
 * Anything that is not a recognisable YouTube thumbnail is returned UNCHANGED
 * rather than rejected: artwork comes from a lookup that may one day return
 * something else, and a URL this function does not understand is still a URL the
 * proxy may well be able to serve.
 */
export function ytdlThumbUrl(artwork: string | undefined, size: YtdlThumbSize): string {
  if (!artwork) return '';
  let u: URL;
  try {
    u = new URL(artwork);
  } catch {
    return artwork;
  }
  const m = THUMB_PATH.exec(u.pathname);
  if (!m) return artwork;
  u.pathname = `/${m[1]}/${m[2]}/${FILENAME[size]}`;
  return u.toString();
}

/**
 * The same URL, addressed through SynoDL's artwork proxy.
 *
 * The proxy is why a viewer's browser never contacts Google (spec 1034), so no
 * surface should ever put a raw thumbnail URL in an `<img src>`.
 */
export function ytdlThumbSrc(artwork: string | undefined, size: YtdlThumbSize): string {
  const url = ytdlThumbUrl(artwork, size);
  return url ? `/v1/ytdl/thumb?u=${encodeURIComponent(url)}` : '';
}
