<script setup lang="ts">
import { IonItem, IonLabel, IonList, IonListHeader, IonNote, IonToggle } from '@ionic/vue';
import { onMounted, ref } from 'vue';
import { api } from '@/services/api';

const supported =
  'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window;

const enabled = ref(false);
const busy = ref(false);
const error = ref('');
const iosNeedsInstall = ref(false);

// VAPID public key (base64url) → the Uint8Array the Push API wants.
function urlB64ToUint8Array(base64url: string): Uint8Array {
  const padding = '='.repeat((4 - (base64url.length % 4)) % 4);
  const b64 = (base64url + padding).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(b64);
  const arr = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) arr[i] = raw.charCodeAt(i);
  return arr;
}

async function currentSub(): Promise<PushSubscription | null> {
  const reg = await navigator.serviceWorker.ready;
  return reg.pushManager.getSubscription();
}

onMounted(async () => {
  if (!supported) return;
  // iOS delivers Web Push only to an installed (home-screen) PWA.
  const standalone =
    window.matchMedia('(display-mode: standalone)').matches ||
    (navigator as unknown as { standalone?: boolean }).standalone === true;
  const isIOS = /iP(hone|ad|od)/.test(navigator.userAgent);
  if (isIOS && !standalone) {
    iosNeedsInstall.value = true;
    return;
  }
  try {
    enabled.value = (await currentSub()) !== null && Notification.permission === 'granted';
  } catch {
    /* leave off */
  }
});

async function onToggle(ev: CustomEvent): Promise<void> {
  const want = (ev.detail as { checked: boolean }).checked;
  if (want === enabled.value) return;
  busy.value = true;
  error.value = '';
  try {
    if (want) await subscribe();
    else await unsubscribe();
    enabled.value = want;
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Could not update notifications.';
    enabled.value = !want; // revert the toggle
  } finally {
    busy.value = false;
  }
}

async function subscribe(): Promise<void> {
  const perm = await Notification.requestPermission();
  if (perm !== 'granted') throw new Error('Notification permission was not granted.');
  const { publicKey } = await api.pushKey();
  const reg = await navigator.serviceWorker.ready;
  const sub =
    (await reg.pushManager.getSubscription()) ??
    (await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlB64ToUint8Array(publicKey) as unknown as BufferSource,
    }));
  const j = sub.toJSON();
  if (!j.keys?.p256dh || !j.keys?.auth) throw new Error('Subscription keys unavailable.');
  await api.saveSubscription(sub.endpoint, { p256dh: j.keys.p256dh, auth: j.keys.auth }, true);
}

async function unsubscribe(): Promise<void> {
  const sub = await currentSub();
  if (!sub) return;
  await api.deleteSubscription(sub.endpoint).catch(() => undefined);
  await sub.unsubscribe().catch(() => undefined);
}
</script>

<template>
  <ion-list v-if="supported" inset>
    <ion-list-header><ion-label>Notifications</ion-label></ion-list-header>
    <ion-item v-if="iosNeedsInstall" lines="none">
      <ion-note>Add SynoDL to your Home Screen to enable download notifications on iOS.</ion-note>
    </ion-item>
    <ion-item v-else>
      <ion-toggle
        :checked="enabled"
        :disabled="busy"
        data-testid="push-optin"
        @ion-change="onToggle"
      >
        Notify me on this device when a download finishes
      </ion-toggle>
    </ion-item>
    <ion-note v-if="error" color="danger" class="err" data-testid="push-error">{{ error }}</ion-note>
  </ion-list>
</template>

<style scoped>
.err {
  display: block;
  margin: 0.5rem 1rem;
}
</style>
