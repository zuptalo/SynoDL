import { describe, expect, it } from 'vitest';
import {
  isYoutubeRefusal,
  shouldHintSignIn,
  signInErrorMessage,
  signInSubtitle,
} from './youtube-signin';

describe('signInErrorMessage', () => {
  it('maps each server code to its own wording', () => {
    const all = ['too_few_cookies', 'unrecognised', 'too_large', 'invalid', 'other'].map(
      signInErrorMessage,
    );
    expect(new Set(all).size).toBe(5);
  });
});

describe('signInSubtitle', () => {
  it('reads as a short fact', () => {
    expect(signInSubtitle(null)).toBe('');
    expect(signInSubtitle({ saved: false, cookieCount: 0 })).toBe('Not set');
    expect(signInSubtitle({ saved: true, cookieCount: 24 })).toBe('Saved · 24 cookies');
    expect(signInSubtitle({ saved: true, cookieCount: 1 })).toBe('Saved · 1 cookie');
  });
});

describe('isYoutubeRefusal', () => {
  it('matches both refusal reasons and nothing else', () => {
    expect(isYoutubeRefusal('YouTube turned the request away — trying again later usually works')).toBe(true);
    expect(isYoutubeRefusal('YouTube turned the request away — trying again automatically')).toBe(true);
    expect(isYoutubeRefusal('the saved YouTube sign-in appears to have stopped working — paste a fresh one in Settings')).toBe(false);
    expect(isYoutubeRefusal('this video was removed')).toBe(false);
    expect(isYoutubeRefusal(undefined)).toBe(false);
  });
});

describe('shouldHintSignIn', () => {
  const refused = 'YouTube turned the request away — trying again later usually works';
  it('needs an admin, a refusal, and no sign-in saved', () => {
    expect(shouldHintSignIn(refused, true, { available: true, saved: false })).toBe(true);
    expect(shouldHintSignIn(refused, false, { available: true, saved: false })).toBe(false);
    expect(shouldHintSignIn(refused, true, { available: true, saved: true })).toBe(false);
    expect(shouldHintSignIn(refused, true, { available: false, saved: false })).toBe(false);
    expect(shouldHintSignIn(refused, true, null)).toBe(false);
    expect(shouldHintSignIn('removed', true, { available: true, saved: false })).toBe(false);
  });
});
