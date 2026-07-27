<script setup lang="ts">
import { IonApp, IonRouterOutlet, IonToast } from '@ionic/vue';
import { onUnmounted } from 'vue';
import { useRouter } from 'vue-router';
import { SESSION_EXPIRED_EVENT } from '@/services/api';
import { useAppUpdate } from '@/composables/useAppUpdate';

// Prompt-based updates: the toast names the waiting version; applying is the
// user's call, never automatic (constitution Principle V).
const { updateAvailable, applyUpdate } = useAppUpdate();

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

onUnmounted(() => {
  window.removeEventListener(SESSION_EXPIRED_EVENT, onExpired);
  document.removeEventListener('visibilitychange', clearBadge);
  window.removeEventListener('focus', clearBadge);
});
</script>

<template>
  <ion-app>
    <ion-router-outlet />
    <ion-toast
      :is-open="updateAvailable"
      message="A new version of SynoDL is ready."
      position="top"
      :buttons="[{ text: 'Update', handler: () => applyUpdate() }, { text: 'Later', role: 'cancel' }]"
    />
  </ion-app>
</template>
