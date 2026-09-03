/**
 * Install gate (spec 1008). SynoDL is meant to run as an installed PWA
 * (standalone display mode) — that's what unlocks reliable Web Push and an
 * app-like shell. Opened in a plain browser tab, the app is blocked behind an
 * install guide (components/InstallGuard.vue) so the user can't sign in until
 * they've installed it.
 *
 * Two exceptions. Localhost is allowed un-installed so local development and the
 * e2e suite aren't blocked. And an operator can set ALLOW_BROWSER_ACCESS on the
 * server to lift the gate entirely — for looking at the app from a desktop
 * browser, or debugging without installing. It is off unless turned on: the
 * default stays "install first", because that is what makes Web Push and the app
 * shell reliable.
 */
import { computed, ref } from 'vue';
import { api } from '@/services/api';
import {
  detectPlatform,
  isAndroidWebView,
  isFirefoxAndroid,
  type InstallPlatform,
} from '@/services/install-detect';

interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>;
}

function isStandalone(): boolean {
  try {
    return (
      window.matchMedia('(display-mode: standalone)').matches ||
      window.matchMedia('(display-mode: fullscreen)').matches ||
      window.matchMedia('(display-mode: minimal-ui)').matches ||
      (navigator as Navigator & { standalone?: boolean }).standalone === true
    );
  } catch {
    return false;
  }
}

function isLocalhost(): boolean {
  const h = window.location.hostname;
  return h === 'localhost' || h === '127.0.0.1' || h === '::1' || h === '[::1]';
}

// Remembered across loads so a browser-access instance does not flash the
// install screen on every visit while the config request is in flight. The
// server is still asked every time; this only decides the first frame.
const BROWSER_OK_KEY = 'synodl.browserAccess';

function cachedBrowserAllowed(): boolean {
  try {
    return localStorage.getItem(BROWSER_OK_KEY) === '1';
  } catch {
    return false; // private mode: fall back to enforcing the gate
  }
}

// Singleton state shared across the (single) guard component.
// blocked = "not installed, and not a localhost origin". browserAllowed is the
// operator's opt-out, learned from the server.
const blocked = ref(false);
const browserAllowed = ref(false);
const mustInstall = computed(() => blocked.value && !browserAllowed.value);
const platform = ref<InstallPlatform>('desktop');
const canPrompt = ref(false);
const installUnavailable = ref(false); // Android embedded WebView — truly can't install
const firefoxAndroid = ref(false);
let deferredPrompt: BeforeInstallPromptEvent | null = null;
let started = false;

function start(): void {
  if (started) return;
  started = true;
  const ua = navigator.userAgent || '';
  platform.value = detectPlatform(ua, 'ontouchend' in document);
  browserAllowed.value = cachedBrowserAllowed();
  blocked.value = !isStandalone() && !isLocalhost();

  // Ask the server whether the gate applies here. A failure leaves the gate UP:
  // the safe default is to insist on installing, never to let the app through
  // because a request did not answer.
  void api
    .config()
    .then((c) => {
      const allowed = c.allowBrowserAccess === true;
      browserAllowed.value = allowed;
      try {
        localStorage.setItem(BROWSER_OK_KEY, allowed ? '1' : '0');
      } catch {
        /* private mode: just don't remember it */
      }
    })
    .catch(() => {
      /* keep whatever the cache said; the gate stays up by default */
    });

  if (platform.value === 'android' && mustInstall.value) {
    installUnavailable.value = isAndroidWebView(ua);
    firefoxAndroid.value = isFirefoxAndroid(ua);
  }

  try {
    window
      .matchMedia('(display-mode: standalone)')
      .addEventListener('change', (e: MediaQueryListEvent) => {
        if (e.matches) blocked.value = false;
      });
  } catch {
    /* Safari < 14 lacks addEventListener on MediaQueryList; ignore. */
  }

  window.addEventListener('beforeinstallprompt', (e: Event) => {
    e.preventDefault();
    deferredPrompt = e as BeforeInstallPromptEvent;
    canPrompt.value = true;
    installUnavailable.value = false; // a real install IS possible here
  });
  window.addEventListener('appinstalled', () => {
    deferredPrompt = null;
    canPrompt.value = false;
    blocked.value = false;
  });
}

/** Trigger the native install prompt (Chromium only). No-op otherwise. */
export async function promptInstall(): Promise<void> {
  const e = deferredPrompt;
  if (!e) return;
  deferredPrompt = null;
  canPrompt.value = false;
  try {
    await e.prompt();
    await e.userChoice;
  } catch {
    /* user dismissed / unsupported */
  }
}

export function useInstallGuard() {
  start();
  return { mustInstall, platform, canPrompt, installUnavailable, firefoxAndroid };
}
