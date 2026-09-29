/**
 * How the YouTube sign-in screen words things (spec 1055).
 *
 * Pure on purpose, and pure about a second thing too: nothing here ever receives
 * the pasted text. The server's error codes and counts are all this module sees,
 * so no message it builds can echo a cookie back at the screen.
 */
import type { YoutubeSignInStatus } from '@/services/api';

/** The server's 400 codes for a paste it would not keep, in friendly words. */
export function signInErrorMessage(code: string): string {
  switch (code) {
    case 'too_few_cookies':
      return 'That looks too short to be a real sign-in. Copy the whole Cookie header value.';
    case 'unrecognised':
      return 'That does not look like a Cookie header or a cookies.txt file.';
    case 'too_large':
      return 'That is too large. Paste only the Cookie header value, not the whole request.';
    case 'invalid':
      return 'That could not be read as a sign-in. Copy it again and paste it here.';
    default:
      return 'Could not save the sign-in. Try again in a moment.';
  }
}

/** The warning shown after a save the server accepted but doubts. */
export const NO_LOGIN_WARNING =
  'Saved, but no login cookies were found. This was probably copied from a request made while ' +
  'signed out, so downloads may still be turned away. Sign in first, then copy it again.';

/** The Settings row's subtitle: a short fact, never a value. */
export function signInSubtitle(s: Pick<YoutubeSignInStatus, 'saved' | 'cookieCount'> | null): string {
  if (!s) return '';
  if (!s.saved) return 'Not set';
  return `Saved · ${s.cookieCount} ${s.cookieCount === 1 ? 'cookie' : 'cookies'}`;
}

/**
 * True for a failure reason that is YouTube turning a request away — the only
 * kind a saved sign-in can fix. A permanent fact about a video (removed,
 * age-gated) is not one, and neither is the sign-in itself being refused, which
 * has its own reason and its own advice.
 */
export function isYoutubeRefusal(reason: string | undefined | null): boolean {
  return !!reason && reason.startsWith('YouTube turned the request away');
}

/** Whether to hint that a sign-in could help: a refusal, and none saved yet. */
export function shouldHintSignIn(
  reason: string | undefined | null,
  isAdmin: boolean,
  status: Pick<YoutubeSignInStatus, 'available' | 'saved'> | null,
): boolean {
  return isAdmin && isYoutubeRefusal(reason) && !!status && status.available && !status.saved;
}
