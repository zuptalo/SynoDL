<script setup lang="ts">
/**
 * A YouTube sign-in for the download workers (spec 1055).
 *
 * What is pasted here is a LIVE LOGIN for a Google account, so the rules are
 * about where it is allowed to be: in this component's memory while the admin
 * types, in one PUT body, and nowhere else. It is never written to IndexedDB or
 * localStorage, never logged, never put in a toast or an error message, and it
 * is cleared after a successful save and whenever the sheet closes. The server
 * answers with counts and names only, so there is nothing to show back.
 */
import { computed, ref, watch } from 'vue';
import {
  alertController,
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonItem,
  IonList,
  IonModal,
  IonNote,
  IonTextarea,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { api, ApiError, type YoutubeSignInStatus } from '@/services/api';
import { appToast } from '@/services/toast';
import { formatTimestamp } from '@/utils/format';
import {
  NO_LOGIN_WARNING,
  signInErrorMessage,
  signInSubtitle,
} from '@/services/youtube-signin';

const props = defineProps<{ isOpen: boolean }>();
const emit = defineEmits<{
  (e: 'dismiss'): void;
  (e: 'changed', status: YoutubeSignInStatus): void;
}>();

const status = ref<YoutubeSignInStatus | null>(null);
const text = ref('');
const loading = ref(false);
const saving = ref(false);
const loadError = ref('');
const saveError = ref('');
const warning = ref('');
const savedNote = ref('');
const fileInput = ref<HTMLInputElement | null>(null);

async function load(): Promise<void> {
  loading.value = true;
  loadError.value = '';
  try {
    status.value = await api.getYoutubeSignIn();
  } catch {
    loadError.value = 'Could not read the current sign-in status.';
  } finally {
    loading.value = false;
  }
}

function clearPaste(): void {
  text.value = '';
  if (fileInput.value) fileInput.value.value = '';
}

watch(
  () => props.isOpen,
  (open) => {
    if (open) {
      saveError.value = '';
      warning.value = '';
      savedNote.value = '';
      void load();
    } else {
      // Closing forgets the paste: a login should not wait in memory for the
      // next time somebody opens the sheet.
      clearPaste();
    }
  },
  { immediate: true },
);

/** A cookies.txt read from disk lands in the same field as a paste. */
async function onFile(ev: Event): Promise<void> {
  const file = (ev.target as HTMLInputElement).files?.[0];
  if (!file) return;
  try {
    text.value = await file.text();
  } catch {
    saveError.value = 'Could not read that file.';
  }
}

const canSave = computed(() => text.value.trim().length > 0 && !saving.value && !loading.value);

async function save(): Promise<void> {
  saving.value = true;
  saveError.value = '';
  warning.value = '';
  savedNote.value = '';
  try {
    const res = await api.saveYoutubeSignIn(text.value);
    clearPaste();
    status.value = res;
    savedNote.value = `Saved ${res.cookieCount} cookies, ${res.loginCookies.length} of them login cookies.`;
    if (res.warning === 'no_login_cookies') warning.value = NO_LOGIN_WARNING;
    emit('changed', res);
  } catch (e) {
    // The code only. Nothing about the failure is built from the paste.
    saveError.value = signInErrorMessage(e instanceof ApiError ? e.code : '');
  } finally {
    saving.value = false;
  }
}

async function remove(): Promise<void> {
  const alert = await alertController.create({
    header: 'Remove the YouTube sign-in?',
    message: 'Downloads started afterwards will run without it.',
    buttons: [
      { text: 'Cancel', role: 'cancel' },
      { text: 'Remove', role: 'destructive' },
    ],
  });
  await alert.present();
  const { role } = await alert.onDidDismiss();
  if (role !== 'destructive') return;
  try {
    await api.removeYoutubeSignIn();
    savedNote.value = '';
    warning.value = '';
    await load();
    if (status.value) emit('changed', status.value);
    await appToast({ message: 'YouTube sign-in removed.', duration: 1800 });
  } catch {
    await appToast({ message: 'Could not remove it. Try again.', color: 'danger', duration: 2600 });
  }
}
</script>

<template>
  <ion-modal :is-open="isOpen" @did-dismiss="emit('dismiss')">
    <ion-header>
      <ion-toolbar>
        <ion-title>YouTube sign-in</ion-title>
        <ion-buttons slot="end">
          <ion-button data-testid="youtube-signin-close" @click="emit('dismiss')">Close</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content class="ion-padding" data-testid="youtube-signin-modal">
      <ion-note v-if="loadError" color="danger">{{ loadError }}</ion-note>

      <div class="status" data-testid="youtube-signin-status">
        <template v-if="status && status.saved">
          <strong>{{ signInSubtitle(status) }}</strong>
          <div>
            Login cookies found:
            {{ status.loginCookies.length ? status.loginCookies.join(', ') : 'none' }}
          </div>
          <div v-if="status.missingLogin.length">
            Missing: {{ status.missingLogin.join(', ') }}
          </div>
          <div>
            Saved {{ formatTimestamp(status.savedAt) }}<template v-if="status.savedBy">
              by {{ status.savedBy }}</template>
          </div>
          <div v-if="status.lastOkAt">Last worked: {{ formatTimestamp(status.lastOkAt) }}</div>
          <div v-if="status.lastRefusedAt">
            Last refused: {{ formatTimestamp(status.lastRefusedAt) }}
          </div>
        </template>
        <template v-else-if="status">Not saved. Downloads run without a sign-in.</template>
      </div>

      <ion-note class="help warn-note">
        This is a live login for a Google account. It is stored encrypted, only used to start
        YouTube downloads, and never shown again. Use a spare account if you can.
      </ion-note>

      <ion-list>
        <ion-item>
          <ion-textarea
            v-model="text"
            label="Cookie header or cookies.txt"
            label-placement="stacked"
            :rows="5"
            autocomplete="off"
            autocapitalize="off"
            autocorrect="off"
            :spellcheck="false"
            data-lpignore="true"
            data-1p-ignore="true"
            data-testid="youtube-signin-text"
            placeholder="Paste here"
          />
        </ion-item>
      </ion-list>

      <input
        ref="fileInput"
        type="file"
        accept=".txt,text/plain"
        class="file"
        data-testid="youtube-signin-file"
        @change="onFile"
      />

      <ion-note v-if="saveError" color="danger" class="msg" data-testid="youtube-signin-error">
        {{ saveError }}
      </ion-note>
      <ion-note v-if="savedNote" color="success" class="msg" data-testid="youtube-signin-saved">
        {{ savedNote }}
      </ion-note>
      <ion-note v-if="warning" color="warning" class="msg" data-testid="youtube-signin-warning">
        {{ warning }}
      </ion-note>

      <ion-button
        expand="block"
        class="go"
        :disabled="!canSave"
        data-testid="youtube-signin-save"
        @click="save"
      >
        Save
      </ion-button>
      <ion-button
        v-if="status && status.saved"
        expand="block"
        fill="outline"
        color="danger"
        data-testid="youtube-signin-remove"
        @click="remove"
      >
        Remove
      </ion-button>

      <ion-note class="help">
        To get it: open a private window, sign in to YouTube, then open DevTools and choose
        Network. Reload, click any request to www.youtube.com, find Request Headers, and copy
        the value of <strong>Cookie</strong>. Paste it above. A cookies.txt file works too.
      </ion-note>
    </ion-content>
  </ion-modal>
</template>

<style scoped>
.status {
  font-size: 0.9rem;
  line-height: 1.5;
  margin: 0 0.25rem 0.75rem;
}
.help {
  display: block;
  margin: 0.75rem 0.25rem;
  font-size: 0.8rem;
  line-height: 1.4;
}
.msg {
  display: block;
  margin: 0.5rem 0.25rem;
  font-size: 0.85rem;
}
.file {
  display: block;
  margin: 0.5rem 0.25rem;
  max-width: 100%;
  font-size: 0.85rem;
}
.go {
  margin: 1rem 0 0.5rem;
}
</style>
