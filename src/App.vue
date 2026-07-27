<script setup lang="ts">
import { IonApp, IonRouterOutlet, IonToast } from '@ionic/vue';
import { onUnmounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { SESSION_EXPIRED_EVENT } from '@/services/api';
import { useAppUpdate } from '@/composables/useAppUpdate';
import { useTheme } from '@/composables/useTheme';
import UpdateModal from '@/components/UpdateModal.vue';
import InstallGuard from '@/components/InstallGuard.vue';

// Apply the persisted dark/light choice at startup (the index.html pre-paint
// covers the very first frame; this keeps the runtime palette in sync).
useTheme();

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

// Clear the app-icon notification badge whenever the app is in view (the SW set
// it on a push). Progressive enhancement — a silent no-op where unsupported.
const clearBadge = (): void => {
  if (document.visibilityState !== 'visible') return;
  const nav = navigator as Navigator & { clearAppBadge?: () => Promise<void> };
  if (nav.clearAppBadge) void nav.clearAppBadge().catch(() => undefined);
};
clearBadge();
document.addEventListener('visibilitychange', clearBadge);
window.addEventListener('focus', clearBadge);

// In-app notifications (spec 1013): when the app is in the foreground the SW
// forwards a push here instead of showing a system notification. Surface it as a
// toast UNLESS the user is already on the Tasks tab, where the change is visible
// live.
const inAppMsg = ref('');
const onSwMessage = (e: MessageEvent): void => {
  const d = e.data as { type?: string; title?: string; body?: string } | undefined;
  if (!d || d.type !== 'push-notification') return;
  if (router.currentRoute.value.path.startsWith('/tabs/tasks')) return;
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
    <ion-router-outlet />
    <!-- Blocks the app behind an install guide unless running as an installed
         PWA (localhost exempt for dev/e2e). Spec 1008. -->
    <InstallGuard />
    <UpdateModal :is-open="updateAvailable" :applying="applying" @confirm="applyUpdate" />
    <ion-toast
      :is-open="!!inAppMsg"
      :message="inAppMsg"
      :duration="4000"
      position="top"
      data-testid="inapp-notification"
      @didDismiss="inAppMsg = ''"
    />
  </ion-app>
</template>
