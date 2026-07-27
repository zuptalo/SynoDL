import { describe, expect, it } from 'vitest';
import { ApiError } from './api';
import { messageForError } from './syno-errors';

describe('messageForError', () => {
  const cases: Array<[string, string]> = [
    ['credentials', 'Wrong account or password.'],
    ['otp_required', 'This account needs a 2-step verification code.'],
    ['otp_invalid', 'That verification code was not accepted.'],
    ['permission', 'This account is not allowed to use Download Station.'],
    ['nas_unreachable', 'The NAS could not be reached. Is it powered on and connected?'],
    ['session', 'Your session ended. Please sign in again.'],
  ];

  it.each(cases)('maps %s to its own message', (code, message) => {
    expect(messageForError(new ApiError(code, 401))).toBe(message);
  });

  it('every known code maps to a DISTINCT message', () => {
    const messages = cases.map(([code]) => messageForError(new ApiError(code, 401)));
    expect(new Set(messages).size).toBe(messages.length);
  });

  it('unknown ApiError codes get the generic fallback', () => {
    expect(messageForError(new ApiError('nas', 502))).toBe(
      'The NAS reported an error. Please try again.',
    );
    expect(messageForError(new ApiError('http_500', 500))).toBe(
      'The NAS reported an error. Please try again.',
    );
  });

  it('non-ApiError failures read as a connectivity problem', () => {
    expect(messageForError(new TypeError('fetch failed'))).toBe('Could not reach the server.');
    expect(messageForError(undefined)).toBe('Could not reach the server.');
  });
});
