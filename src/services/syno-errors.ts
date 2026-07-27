/**
 * One place turning the server's error contract into user-facing copy
 * (spec 0001 FR-004: every auth failure mode reads distinctly). Pure and on
 * the vitest coverage gate.
 */
import { ApiError } from './api';

const MESSAGES: Record<string, string> = {
  credentials: 'Wrong account or password.',
  otp_required: 'This account needs a 2-step verification code.',
  otp_invalid: 'That verification code was not accepted.',
  permission: 'This account is not allowed to use Download Station.',
  nas_unreachable: 'The NAS could not be reached. Is it powered on and connected?',
  session: 'Your session ended. Please sign in again.',
  // DSM Login Web API Guide codes 401 / 407 / 408-410 (spec 1001) — each has a
  // different remedy in DSM, so each gets its own copy.
  account_disabled: 'This account is disabled in DSM.',
  ip_blocked: "DSM has blocked this device's address after too many failed attempts.",
  password_expired: 'The account password has expired — change it in DSM, then sign in again.',
};

/** Any failure → plain-language message. Non-ApiError means the request never
 *  got an HTTP answer (proxy down, offline). */
export function messageForError(err: unknown): string {
  if (!(err instanceof ApiError)) return 'Could not reach the server.';
  return MESSAGES[err.code] ?? 'The NAS reported an error. Please try again.';
}
