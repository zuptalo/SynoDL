<script setup lang="ts">
import { ref, watch } from 'vue';
import {
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonInput,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonModal,
  IonNote,
  IonSpinner,
  IonTextarea,
  IonTitle,
  IonToolbar,
  toastController,
} from '@ionic/vue';
import { api, ApiError, type SourceStatus } from '@/services/api';

const props = defineProps<{ isOpen: boolean }>();
const emit = defineEmits<{ (e: 'dismiss'): void }>();

const status = ref<SourceStatus | null>(null);
const loading = ref(false);
const saving = ref(false);
const errorMsg = ref('');

// Form fields. Session material is write-only — never prefilled from the server.
const displayName = ref('30nama');
const moviesParent = ref('');
const tvParent = ref('');
const cfClearance = ref('');
const cApiKey = ref('');
const cToken = ref('');
const userAgent = ref('');
const cPlatform = ref('');
const cAppVersion = ref('');

const KIND = 'thirtynama';

watch(
  () => props.isOpen,
  async (open) => {
    if (!open) return;
    errorMsg.value = '';
    await refresh();
  },
);

async function refresh(): Promise<void> {
  loading.value = true;
  try {
    const s = await api.getSourceStatus();
    status.value = s;
    // Prefill the non-secret config so it's clear what's stored. Secret fields
    // stay blank by design — the server never returns them.
    if (s.configured) {
      if (s.providerName) displayName.value = s.providerName;
      moviesParent.value = s.moviesParent;
      tvParent.value = s.tvParent;
    }
  } catch {
    status.value = null;
  } finally {
    loading.value = false;
  }
}

function stateLabel(s: SourceStatus | null): string {
  if (!s || !s.configured) return 'Not configured';
  if (s.state === 'needs_refresh') return 'Needs refreshing';
  if (s.state === 'active') return 'Active';
  return s.state;
}

async function toast(message: string): Promise<void> {
  const t = await toastController.create({ message, duration: 2500, position: 'bottom' });
  await t.present();
}

async function save(): Promise<void> {
  errorMsg.value = '';
  if (!moviesParent.value.trim() || !cToken.value.trim() || !cfClearance.value.trim() || !userAgent.value.trim()) {
    errorMsg.value = 'Movies folder, clearance cookie, auth token, and User-Agent are required.';
    return;
  }
  saving.value = true;
  try {
    await api.putSourceSession({
      kind: KIND,
      displayName: displayName.value.trim(),
      moviesParent: moviesParent.value.trim(),
      tvParent: tvParent.value.trim(),
      session: {
        cfClearance: cfClearance.value.trim(),
        cApiKey: cApiKey.value.trim(),
        cToken: cToken.value.trim(),
        userAgent: userAgent.value.trim(),
        cPlatform: cPlatform.value.trim(),
        cAppVersion: cAppVersion.value.trim(),
      },
    });
    await toast('Source verified and saved.');
    // Clear the secret fields from memory once accepted.
    cfClearance.value = cApiKey.value = cToken.value = '';
    await refresh();
  } catch (e) {
    if (e instanceof ApiError && e.code === 'provider_verify_failed') {
      errorMsg.value = 'Verification failed — the session may be expired or the IP no longer matches. Re-capture and try again.';
    } else if (e instanceof ApiError && e.code === 'unknown_provider') {
      errorMsg.value = 'That provider is not supported.';
    } else {
      errorMsg.value = 'Could not save the source.';
    }
  } finally {
    saving.value = false;
  }
}

async function remove(): Promise<void> {
  saving.value = true;
  errorMsg.value = '';
  try {
    await api.deleteSourceSession();
    await toast('Source removed.');
    await refresh();
  } catch {
    errorMsg.value = 'Could not remove the source.';
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <ion-modal :is-open="isOpen" @didDismiss="emit('dismiss')">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>Download source</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="emit('dismiss')">Close</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content class="ion-padding">
      <div v-if="loading" class="centered"><ion-spinner /></div>

      <template v-else>
        <ion-list :inset="true">
          <ion-list-header>Status</ion-list-header>
          <ion-item>
            <ion-label>State</ion-label>
            <ion-note slot="end" :color="status?.state === 'active' ? 'success' : status?.state === 'needs_refresh' ? 'warning' : 'medium'">
              {{ stateLabel(status) }}
            </ion-note>
          </ion-item>
          <ion-item v-if="status?.configured">
            <ion-label>Provider</ion-label>
            <ion-note slot="end">{{ status?.providerName }}</ion-note>
          </ion-item>
        </ion-list>

        <ion-list :inset="true">
          <ion-list-header>Destination folders</ion-list-header>
          <ion-item>
            <ion-input
              v-model="moviesParent"
              label="Movies parent"
              label-placement="stacked"
              placeholder="e.g. movie or video/movies"
            />
          </ion-item>
          <ion-item>
            <ion-input
              v-model="tvParent"
              label="TV / series parent"
              label-placement="stacked"
              placeholder="e.g. tv-show (used later)"
            />
          </ion-item>
        </ion-list>

        <ion-list :inset="true">
          <ion-list-header>Session material</ion-list-header>
          <ion-note class="hint">
            Capture these from your logged-in browser on the same network as the NAS. They are stored
            encrypted and never shown again.
          </ion-note>
          <ion-item>
            <ion-input v-model="displayName" label="Display name" label-placement="stacked" />
          </ion-item>
          <ion-item>
            <ion-textarea
              v-model="cfClearance"
              label="cf_clearance cookie"
              label-placement="stacked"
              :auto-grow="true"
              :rows="2"
            />
          </ion-item>
          <ion-item>
            <ion-textarea
              v-model="cToken"
              label="c-token"
              label-placement="stacked"
              :auto-grow="true"
              :rows="2"
            />
          </ion-item>
          <ion-item>
            <ion-input v-model="cApiKey" label="c-api-key" label-placement="stacked" />
          </ion-item>
          <ion-item>
            <ion-textarea
              v-model="userAgent"
              label="User-Agent"
              label-placement="stacked"
              :auto-grow="true"
              :rows="2"
            />
          </ion-item>
          <ion-item>
            <ion-input v-model="cPlatform" label="c-platform (optional)" label-placement="stacked" />
          </ion-item>
          <ion-item>
            <ion-input v-model="cAppVersion" label="c-app-version (optional)" label-placement="stacked" />
          </ion-item>
        </ion-list>

        <ion-note v-if="errorMsg" color="danger" class="error">{{ errorMsg }}</ion-note>

        <ion-button expand="block" :disabled="saving" @click="save">
          <ion-spinner v-if="saving" slot="start" name="crescent" />
          Verify &amp; Save
        </ion-button>
        <ion-button
          v-if="status?.configured"
          expand="block"
          fill="clear"
          color="danger"
          :disabled="saving"
          @click="remove"
        >
          Remove source
        </ion-button>
      </template>
    </ion-content>
  </ion-modal>
</template>

<style scoped>
.centered {
  display: flex;
  justify-content: center;
  padding-top: 30vh;
}
.hint {
  display: block;
  padding: 0 16px 8px;
}
.error {
  display: block;
  margin: 8px 4px;
}
</style>
