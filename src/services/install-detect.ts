/**
 * Pure user-agent detection for the PWA install guard (spec 1008). Kept
 * separate from the DOM-touching composable so it's deterministic and on the
 * vitest coverage gate.
 */

export type InstallPlatform = 'ios' | 'android' | 'desktop';

/**
 * Coarse platform for install guidance. iPadOS reports as "Macintosh", so a
 * touch-capable Mac is treated as iOS (the caller passes whether touch exists).
 */
export function detectPlatform(ua: string, macHasTouch: boolean): InstallPlatform {
  if (/iphone|ipad|ipod/i.test(ua) || (/Macintosh/.test(ua) && macHasTouch)) return 'ios';
  if (/android/i.test(ua)) return 'android';
  return 'desktop';
}

/**
 * Whether an Android UA is an embedded WebView — the one common Android surface
 * that genuinely cannot install a PWA (no "Install app" menu). We do NOT infer
 * this from a missing `beforeinstallprompt` (capable Chrome can fire it late).
 * The modern WebView tags "; wv)"; the legacy signature is a "Version/x" token
 * alongside "Chrome/" (real Chrome/Samsung/Edge carry no "Version/").
 */
export function isAndroidWebView(ua: string): boolean {
  if (!/android/i.test(ua)) return false;
  if (/;\s*wv[)]/i.test(ua)) return true;
  return /\bVersion\/[\d.]+/i.test(ua) && /\bChrome\/[\d.]+/i.test(ua);
}

/**
 * Whether this is Firefox on Android, which can't install a PWA as a real
 * standalone app (no beforeinstallprompt; "Add to Home" only makes a shortcut).
 */
export function isFirefoxAndroid(ua: string): boolean {
  return /android/i.test(ua) && /\bFirefox\/[\d.]+/i.test(ua);
}
