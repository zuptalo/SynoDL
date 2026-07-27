<script setup lang="ts">
import {
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonNote,
  IonSegment,
  IonSegmentButton,
  IonToggle,
} from '@ionic/vue';
import { onMounted, ref } from 'vue';
import { api, type NotifPrefs } from '@/services/api';

const supported =
  'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window;

const enabled = ref(false);
const busy = ref(false);
const error = ref('');
const iosNeedsInstall = ref(false);

// Per-user notification preferences (which events, whose tasks).
const prefs = ref<NotifPrefs>({
  notifyAdded: false,
  notifyCompleted: true,
  notifyFailed: true,
  scope: 'own',
});

async function loadPrefs(): Promise<void> {
  try {
    prefs.value = await api.getNotifPrefs();
  } catch {
    /* keep defaults */
  }
}

async function savePrefs(): Promise<void> {
  try {
    await api.setNotifPrefs(prefs.value);
  } catch {
    error.value = 'Could not save notification preferences.';
  }
}

function setEvent(key: 'notifyAdded' | 'notifyCompleted' | 'notifyFailed', ev: CustomEvent): void {
  prefs.value = { ...prefs.value, [key]: (ev.detail as { checked: boolean }).checked };
  void savePrefs();
}
function setScope(ev: CustomEvent): void {
  const value = (ev.detail as { value?: string }).value;
  prefs.value = { ...prefs.value, scope: value === 'any' ? 'any' : 'own' };
  void savePrefs();
}

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
  await loadPrefs();
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
      <ion-toggle :checked="enabled" :disabled="busy" data-testid="push-optin" @ion-change="onToggle">
        Notify me on this device
      </ion-toggle>
    </ion-item>

    <!-- What to be notified about + whose tasks (spec 1004). Shown once enabled. -->
    <template v-if="enabled && !iosNeedsInstall">
      <ion-item>
        <ion-toggle
          :checked="prefs.notifyAdded"
          data-testid="notif-added"
          @ion-change="(e) => setEvent('notifyAdded', e)"
        >
          When a download is added
        </ion-toggle>
      </ion-item>
      <ion-item>
        <ion-toggle
          :checked="prefs.notifyCompleted"
          data-testid="notif-completed"
          @ion-change="(e) => setEvent('notifyCompleted', e)"
        >
          When a download finishes
        </ion-toggle>
      </ion-item>
      <ion-item>
        <ion-toggle
          :checked="prefs.notifyFailed"
          data-testid="notif-failed"
          @ion-change="(e) => setEvent('notifyFailed', e)"
        >
          When a download fails
        </ion-toggle>
      </ion-item>
      <ion-item lines="none">
        <ion-label>
          <p>For</p>
        </ion-label>
        <ion-segment :value="prefs.scope" data-testid="notif-scope" @ion-change="setScope">
          <ion-segment-button value="own"><ion-label>My downloads</ion-label></ion-segment-button>
          <ion-segment-button value="any"><ion-label>Everyone's</ion-label></ion-segment-button>
        </ion-segment>
      </ion-item>
    </template>

    <ion-note v-if="error" color="danger" class="err" data-testid="push-error">{{ error }}</ion-note>
  </ion-list>
</template>

<style scoped>
.err {
  display: block;
  margin: 0.5rem 1rem;
}
</style>
