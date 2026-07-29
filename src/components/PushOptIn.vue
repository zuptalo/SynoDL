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
import { useSession } from '@/composables/useSession';

// Only admins choose scope: a regular user always sees and is notified about just
// their own downloads. Admins default to everyone's and can narrow to their own.
const { isAdmin } = useSession();

const supported =
  'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window;

// Remembers that the user opted in on this device. iOS drops the push
// subscription when the installed PWA updates, which would silently turn
// notifications off until the user noticed and re-enabled them. With this flag
// we re-subscribe automatically on next launch (permission is still granted),
// so notifications survive updates and restarts.
const OPTED_IN_KEY = 'push.optedIn';
function rememberOptIn(on: boolean): void {
  try {
    if (on) localStorage.setItem(OPTED_IN_KEY, '1');
    else localStorage.removeItem(OPTED_IN_KEY);
  } catch {
    /* private mode — the in-memory state still drives this session */
  }
}
function wasOptedIn(): boolean {
  try {
    return localStorage.getItem(OPTED_IN_KEY) === '1';
  } catch {
    return false;
  }
}

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
  // Load prefs first so an admin's scope choice is available even before (or
  // without) enabling push — it also governs what they see in the Tasks list.
  await loadPrefs();
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
    const granted = Notification.permission === 'granted';
    const sub = await currentSub();
    if (sub && granted) {
      enabled.value = true;
    } else if (granted && wasOptedIn()) {
      // The subscription was dropped (typically by an iOS PWA update) but the
      // user still wants notifications and permission is granted — restore it
      // silently so they don't have to re-enable after every update.
      try {
        await subscribe();
        enabled.value = true;
      } catch {
        enabled.value = false;
      }
    }
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
  rememberOptIn(true);
}

async function unsubscribe(): Promise<void> {
  // The user explicitly turned it off — don't auto-restore on next launch.
  rememberOptIn(false);
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
      <ion-item :lines="isAdmin ? 'full' : 'none'">
        <ion-toggle
          :checked="prefs.notifyFailed"
          data-testid="notif-failed"
          @ion-change="(e) => setEvent('notifyFailed', e)"
        >
          When a download fails
        </ion-toggle>
      </ion-item>
    </template>

    <!-- Admin-only: whose downloads to show and notify about. Non-admins always
         see and hear about only their own, so there's no choice to offer them.
         This governs both the Tasks list and notifications. -->
    <template v-if="isAdmin && !iosNeedsInstall">
      <!-- Label on its own full-width line above the segment, so it doesn't get
           squeezed into a narrow column and wrap one word per line. -->
      <ion-item lines="none">
        <ion-label class="scope-label">Show &amp; notify me about</ion-label>
      </ion-item>
      <ion-item>
        <ion-segment :value="prefs.scope" data-testid="notif-scope" @ion-change="setScope">
          <ion-segment-button value="own"><ion-label>My downloads</ion-label></ion-segment-button>
          <ion-segment-button value="any"><ion-label>Everyone's</ion-label></ion-segment-button>
        </ion-segment>
      </ion-item>
      <ion-item lines="none">
        <ion-note class="scope-hint">
          Applies to the Tasks list and notifications. "Everyone's" also shows who added each task.
        </ion-note>
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
.scope-label {
  white-space: normal;
}
.scope-hint {
  font-size: 0.8rem;
  color: var(--app-text-dim);
}
</style>
