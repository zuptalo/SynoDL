import { describe, expect, it } from 'vitest';
import { detectPlatform, isAndroidWebView, isFirefoxAndroid } from './install-detect';

const CHROME_ANDROID =
  'Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Mobile Safari/537.36';
const WEBVIEW_ANDROID =
  'Mozilla/5.0 (Linux; Android 14; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/120.0 Mobile Safari/537.36';
const FIREFOX_ANDROID = 'Mozilla/5.0 (Android 14; Mobile; rv:121.0) Gecko/121.0 Firefox/121.0';
const IPHONE = 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148';
const MAC = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15';

describe('detectPlatform', () => {
  it('detects iPhone as ios', () => {
    expect(detectPlatform(IPHONE, false)).toBe('ios');
  });
  it('treats a touch-capable Mac (iPadOS) as ios', () => {
    expect(detectPlatform(MAC, true)).toBe('ios');
  });
  it('treats a non-touch Mac as desktop', () => {
    expect(detectPlatform(MAC, false)).toBe('desktop');
  });
  it('detects Android', () => {
    expect(detectPlatform(CHROME_ANDROID, false)).toBe('android');
  });
});

describe('isAndroidWebView', () => {
  it('flags the modern "; wv)" WebView', () => {
    expect(isAndroidWebView(WEBVIEW_ANDROID)).toBe(true);
  });
  it('does not flag real Chrome on Android', () => {
    expect(isAndroidWebView(CHROME_ANDROID)).toBe(false);
  });
  it('is false off Android', () => {
    expect(isAndroidWebView(IPHONE)).toBe(false);
  });
});

describe('isFirefoxAndroid', () => {
  it('flags Firefox on Android', () => {
    expect(isFirefoxAndroid(FIREFOX_ANDROID)).toBe(true);
  });
  it('is false for Chrome on Android', () => {
    expect(isFirefoxAndroid(CHROME_ANDROID)).toBe(false);
  });
});
