/**
 * In-app update flow (spec 1003). A new deploy parks the fresh service worker in
 * "waiting" (vite-plugin-pwa registerType:'prompt'). We surface that as an
 * update PAGE with the incoming release notes and a single OK; OK applies the
 * update and reloads. If the user is notified/OK'd but the app is closed before
 * the reload finishes, the next launch finishes the update automatically (a
 * recorded pending-version flag drives that — see decideUpdate).
 */
import { ref, watch } from 'vue';
import { useRegisterSW } from 'virtual:pwa-register/vue';
import { decideUpdate } from '@/services/app-update';

const PENDING_KEY = 'update.pendingVersion';

// The active SW registration (captured on register) + a one-time guard so the
// foreground re-check listeners are only wired once.
let swReg: ServiceWorkerRegistration | undefined;
let foregroundHooked = false;

function readPending(): string | null {
  try {
    return localStorage.getItem(PENDING_KEY);
  } catch {
    return null;
  }
}
function writePending(v: string): void {
  try {
    localStorage.setItem(PENDING_KEY, v);
  } catch {
    /* storage disabled — auto-heal just won't survive a hard exit */
  }
}
function clearPending(): void {
  try {
    localStorage.removeItem(PENDING_KEY);
  } catch {
    /* ignore */
  }
}

export function useAppUpdate() {
  const applying = ref(false);
  const showPage = ref(false);
  const { needRefresh, updateServiceWorker } = useRegisterSW({
    immediate: true,
    onRegisteredSW(_url, r) {
      swReg = r;
    },
  });

  // A backgrounded PWA never re-checks for a new service worker on its own, so
  // tapping the "update available" notification would just resume the app
  // without surfacing the update page until a full relaunch. Re-check every time
  // the app is brought to the foreground so the waiting worker is found and the
  // update page appears in-session.
  if (!foregroundHooked) {
    foregroundHooked = true;
    const recheck = () => {
      if (document.visibilityState === 'visible') void swReg?.update().catch(() => undefined);
    };
    document.addEventListener('visibilitychange', recheck);
    window.addEventListener('focus', recheck);
    // An installed DESKTOP PWA can stay open and focused for days, so it never
    // fires visibilitychange/focus and the browser's own periodic SW check may
    // not run — the update would sit unnoticed until a manual refresh. Poll for a
    // waiting worker on an interval so the update page still appears in-session.
    setInterval(() => void swReg?.update().catch(() => undefined), 15 * 60 * 1000);
  }

  async function applyUpdate(targetVersion?: string): Promise<void> {
    applying.value = true;
    // Record the target BEFORE reloading so an interrupted apply self-heals.
    if (targetVersion) writePending(targetVersion);
    // updateServiceWorker(true) posts SKIP_WAITING and reloads on activation.
    await updateServiceWorker(true);
  }

  function evaluate(waiting: boolean): void {
    const d = decideUpdate({
      waiting,
      runningVersion: __APP_VERSION__,
      pendingVersion: readPending(),
    });
    if (d.clearFlag) clearPending();
    if (d.autoApply) {
      void applyUpdate(); // target already recorded from last session
      return;
    }
    showPage.value = d.prompt;
  }

  // Evaluate now (a completed or interrupted update from last launch) and again
  // whenever the SW reports a freshly waiting worker.
  evaluate(needRefresh.value);
  watch(needRefresh, (w) => evaluate(w));

  // Ask the browser to re-check for a new service worker now. Used when a
  // "new version" push arrives while the app is already foregrounded (no
  // visibilitychange fires), so the waiting worker is found and the update page
  // surfaces in-session instead of only after a full relaunch.
  function checkForUpdate(): void {
    void swReg?.update().catch(() => undefined);
  }

  return { updateAvailable: showPage, applying, applyUpdate, checkForUpdate };
}
