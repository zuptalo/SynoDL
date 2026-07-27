/**
 * Install gate (spec 1008). SynoDL is meant to run as an installed PWA
 * (standalone display mode) — that's what unlocks reliable Web Push and an
 * app-like shell. Opened in a plain browser tab, the app is blocked behind an
 * install guide (components/InstallGuard.vue) so the user can't sign in until
 * they've installed it.
 *
 * Exception: localhost is allowed un-installed so local development and the e2e
 * suite aren't blocked. Real users hit the public origin and must install.
 */
import { ref } from 'vue';
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

// Singleton state shared across the (single) guard component.
const mustInstall = ref(false);
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
  mustInstall.value = !isStandalone() && !isLocalhost();

  if (platform.value === 'android' && mustInstall.value) {
    installUnavailable.value = isAndroidWebView(ua);
    firefoxAndroid.value = isFirefoxAndroid(ua);
  }

  try {
    window
      .matchMedia('(display-mode: standalone)')
      .addEventListener('change', (e: MediaQueryListEvent) => {
        if (e.matches) mustInstall.value = false;
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
    mustInstall.value = false;
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
