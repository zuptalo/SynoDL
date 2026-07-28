<script setup lang="ts">
import { IonApp, IonIcon, IonRouterOutlet, IonToast } from '@ionic/vue';
import { cloudOfflineOutline } from 'ionicons/icons';
import { onUnmounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { SESSION_EXPIRED_EVENT } from '@/services/api';
import { useAppUpdate } from '@/composables/useAppUpdate';
import { useConnectivity } from '@/composables/useConnectivity';
import { useInstallGuard } from '@/composables/useInstallGuard';
import { useTheme } from '@/composables/useTheme';
import { resetBadge } from '@/utils/badge';
import UpdateModal from '@/components/UpdateModal.vue';
import InstallGuard from '@/components/InstallGuard.vue';

// Whether an un-installed browser must install first (spec 1008). While the gate
// is up we DON'T render the router outlet, so the app never mounts its tabs —
// no task polling/stream, no destination prefs, no catalog/poster fetches behind
// the install screen.
const { mustInstall } = useInstallGuard();

// Apply the persisted dark/light choice at startup (the index.html pre-paint
// covers the very first frame; this keeps the runtime palette in sync).
useTheme();

// Whether the SynoDL server is reachable — drives the offline banner.
const { reachable } = useConnectivity();

// A new deploy surfaces a full-screen update page (spec 1003): what's new + a
// single OK that applies and reloads. An interrupted apply finishes on the next
// launch (useAppUpdate).
const { updateAvailable, applying, applyUpdate } = useAppUpdate();

// When the NAS ends the session (any request answering 401 "session"),
// useSession already dropped the sid — this is the navigation half: return to
// login instead of leaving a dead task list (spec 0001 US1 scenario 6).
const router = useRouter();
const onExpired = () => {
  void router.replace('/login');
};
window.addEventListener(SESSION_EXPIRED_EVENT, onExpired);

// Clear the app-icon notification badge AND reset its count whenever the app is
// in view (the SW grows it per push while closed). Progressive enhancement — a
// silent no-op where unsupported.
const clearBadge = (): void => {
  if (document.visibilityState !== 'visible') return;
  void resetBadge();
};
clearBadge();
document.addEventListener('visibilitychange', clearBadge);
window.addEventListener('focus', clearBadge);

// In-app notifications (spec 1013): when the app is in the foreground the SW
// forwards a push here instead of showing a system notification. Surface it as a
// toast UNLESS the user is already on the Tasks tab, where the change is visible
// live.
const inAppMsg = ref('');
const inAppTaskId = ref('');
function openTask(taskId: string): void {
  void router.push({ path: '/tabs/tasks', query: taskId ? { task: taskId } : {} });
}
// Tapping "View" opens the notification's task detail (or the Tasks list when
// there's no task). Swipe up dismisses; the button also dismisses.
const toastButtons = [
  {
    text: 'View',
    handler: (): void => {
      const id = inAppTaskId.value;
      inAppMsg.value = '';
      openTask(id);
    },
  },
];
const onSwMessage = (e: MessageEvent): void => {
  const d = e.data as
    | { type?: string; title?: string; body?: string; taskId?: string }
    | undefined;
  if (!d) return;
  // A tapped OS notification (app already open) — route straight to the task.
  if (d.type === 'open-task') {
    openTask(d.taskId ?? '');
    return;
  }
  if (d.type !== 'push-notification') return;
  if (router.currentRoute.value.path.startsWith('/tabs/tasks')) return;
  inAppTaskId.value = d.taskId ?? '';
  inAppMsg.value = d.body ? `${d.title}: ${d.body}` : (d.title ?? '');
};
navigator.serviceWorker?.addEventListener('message', onSwMessage);

onUnmounted(() => {
  window.removeEventListener(SESSION_EXPIRED_EVENT, onExpired);
  document.removeEventListener('visibilitychange', clearBadge);
  window.removeEventListener('focus', clearBadge);
  navigator.serviceWorker?.removeEventListener('message', onSwMessage);
});
</script>

<template>
  <ion-app>
    <!-- Persistent banner while the server is unreachable, so nothing is a guess.
         Clears automatically on the next successful request. -->
    <div v-if="!reachable" class="offline-banner" role="status">
      <ion-icon :icon="cloudOfflineOutline" />
      Can't reach the server — reconnecting…
    </div>
    <!-- Only mount the app (and thus its network activity) when NOT gated behind
         the install screen. localhost is exempt, so dev/e2e always render. -->
    <ion-router-outlet v-if="!mustInstall" />
    <!-- Blocks the app behind an install guide unless running as an installed
         PWA (localhost exempt for dev/e2e). Spec 1008. -->
    <InstallGuard />
    <UpdateModal :is-open="updateAvailable" :applying="applying" @confirm="applyUpdate" />
    <ion-toast
      class="app-toast"
      :is-open="!!inAppMsg"
      :message="inAppMsg"
      :duration="6000"
      position="top"
      swipe-gesture="vertical"
      :buttons="toastButtons"
      data-testid="inapp-notification"
      @didDismiss="inAppMsg = ''"
    />
  </ion-app>
</template>

<style>
/* In-app notification toast. A themed, coloured pill (the app's primary green by
   default, danger/warning/success tones as needed) with white text, rounded
   corners and a soft shadow — the same in-app notification language as the
   sibling app, instead of Ionic's default always-dark toast. Slides in from the
   top (position="top") and is swipe-to-dismiss. */
ion-toast.app-toast {
  --background: rgba(var(--ion-color-primary-rgb, 16, 185, 129), 0.96);
  --color: #fff;
  --border-radius: 16px;
  --box-shadow: 0 6px 22px rgba(0, 0, 0, 0.28);
  --button-color: #fff;
  font-weight: 500;
}
ion-toast.app-toast::part(button) {
  color: #fff;
  font-weight: 600;
}
ion-toast.app-toast-danger {
  --background: rgba(var(--ion-color-danger-rgb, 235, 68, 90), 0.96);
}
ion-toast.app-toast-warning {
  --background: rgba(var(--ion-color-warning-rgb, 234, 179, 8), 0.97);
  --color: #1a1400;
  --button-color: #1a1400;
}
ion-toast.app-toast-success {
  --background: rgba(var(--ion-color-success-rgb, 34, 197, 94), 0.96);
}

/* Server-unreachable banner. Fixed above everything, respects the safe-area
   inset so it clears the status bar / notch on installed apps. */
.offline-banner {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: calc(env(safe-area-inset-top, 0px) + 6px) 12px 6px;
  font-size: 0.85rem;
  font-weight: 500;
  color: #fff;
  background: var(--ion-color-warning, #c96f12);
}
.offline-banner ion-icon {
  font-size: 1.05rem;
}
</style>
