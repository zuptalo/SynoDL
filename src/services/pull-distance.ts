/**
 * How far the pull has travelled, from ONE source.
 *
 * This exists as a function taking a TRANSFORM STRING, and that narrow signature
 * is the whole point of it rather than an accident of factoring.
 *
 * The pull indicator used to measure itself from two places: the transform Ionic
 * applies to the scroller, and the scroller's own negative `scrollTop` during
 * the browser's rubber-band bounce — `Math.max(shift, -el.scrollTop)`, on the
 * theory that the two modes move different things. They do not. On a phone the
 * bounce moves first, so the shape starts drawing from that; a moment later
 * Ionic's gesture passes its threshold, claims the pull, returns the scroller to
 * zero and starts its transform from nothing. The measurement collapsed, and the
 * droplet restarted from a dot — once, early in the pull.
 *
 * That is not reachable from the e2e suite: desktop Chrome has no elastic
 * overscroll on an inner scroller, so `-scrollTop` is never positive there and
 * the second source sits inert. Verified by restoring it and watching the pull
 * test still pass. So the invariant is held HERE instead, by construction: with
 * only a transform to read, a second source cannot be added by accident — it
 * would have to be passed in.
 */

/** `matrix(a, b, c, d, tx, ty)` — ty is the sixth value. */
const MATRIX = /^matrix\(([^)]*)\)$/;
/** `matrix3d(...)` — ty is the fourteenth of sixteen. */
const MATRIX3D = /^matrix3d\(([^)]*)\)$/;

export function pullDistance(transform: string | null | undefined): number {
  if (!transform) return 0;
  const css = transform.trim();
  if (css === 'none') return 0;

  const m2 = MATRIX.exec(css);
  const m3 = MATRIX3D.exec(css);
  const parts = (m2?.[1] ?? m3?.[1])?.split(',').map((n) => Number(n.trim()));
  if (!parts) return 0;

  const ty = m2 ? parts[5] : parts[13];
  // A pull is downward. Anything else — a transform used for something other
  // than the pull, an unreadable value — is no pull at all rather than a
  // negative one, which would draw the shape upside down.
  return Number.isFinite(ty) && ty > 0 ? ty : 0;
}
