<script setup lang="ts">
import {
  IonButton,
  IonContent,
  IonInput,
  IonItem,
  IonList,
  IonListHeader,
  IonNote,
  IonPage,
  IonSpinner,
  IonToggle,
} from '@ionic/vue';
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { messageForError } from '@/services/syno-errors';
import { useSession } from '@/composables/useSession';

const router = useRouter();
const { completeSetup, prefillNasUrl } = useSession();

// Connection
const publicUrl = ref('');
const nasAddress = ref('');
const nasPort = ref(5001);
const nasTlsVerify = ref(false); // most home NAS use a self-signed cert
const nasAccount = ref('');
const nasPassword = ref('');
const otp = ref('');
// SynoDL admin
const adminUsername = ref('');
const adminPassword = ref('');

const busy = ref(false);
const error = ref('');

onMounted(() => {
  // Prefill the NAS address/port from any legacy SYNO_URL so the operator isn't
  // retyping known details.
  if (!prefillNasUrl.value) return;
  try {
    const u = new URL(prefillNasUrl.value);
    nasAddress.value = u.hostname;
    if (u.port) nasPort.value = Number(u.port);
    nasTlsVerify.value = u.protocol === 'https:' ? nasTlsVerify.value : false;
  } catch {
    // Not a parseable URL — leave the fields blank.
  }
});

const canSubmit = computed(
  () =>
    !busy.value &&
    nasAddress.value.trim() !== '' &&
    nasPort.value > 0 &&
    nasAccount.value.trim() !== '' &&
    nasPassword.value !== '' &&
    adminUsername.value.trim() !== '' &&
    adminPassword.value.length >= 8,
);

async function submit(): Promise<void> {
  busy.value = true;
  error.value = '';
  try {
    await completeSetup({
      publicUrl: publicUrl.value.trim(),
      nasAddress: nasAddress.value.trim(),
      nasPort: Number(nasPort.value),
      nasTlsVerify: nasTlsVerify.value,
      nasAccount: nasAccount.value.trim(),
      nasPassword: nasPassword.value,
      otp: otp.value.trim() || undefined,
      adminUsername: adminUsername.value.trim(),
      adminPassword: adminPassword.value,
    });
    await router.replace('/tabs/tasks');
  } catch (e) {
    error.value = messageForError(e);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <ion-page>
    <ion-content class="ion-padding">
      <div class="setup-wrap">
        <h1>Welcome to SynoDL</h1>
        <p class="intro">First-run setup: connect your NAS and create the admin account.</p>

        <ion-list inset>
          <ion-list-header><ion-note>Your NAS</ion-note></ion-list-header>
          <ion-item>
            <ion-input
              v-model="nasAddress"
              label="NAS address"
              label-placement="stacked"
              placeholder="192.168.1.10 or nas.example.com"
              autocapitalize="off"
              data-testid="setup-nas-address"
            />
          </ion-item>
          <ion-item>
            <ion-input
              v-model.number="nasPort"
              label="DSM port"
              label-placement="stacked"
              type="number"
              inputmode="numeric"
              data-testid="setup-nas-port"
            />
          </ion-item>
          <ion-item>
            <ion-toggle v-model="nasTlsVerify" data-testid="setup-tls-verify">
              Verify the NAS TLS certificate
            </ion-toggle>
          </ion-item>
          <ion-item>
            <ion-input
              v-model="nasAccount"
              label="NAS username"
              label-placement="stacked"
              autocapitalize="off"
              autocomplete="off"
              data-testid="setup-nas-account"
            />
          </ion-item>
          <ion-item>
            <ion-input
              v-model="nasPassword"
              label="NAS password"
              label-placement="stacked"
              type="password"
              autocomplete="off"
              data-testid="setup-nas-password"
            />
          </ion-item>
          <ion-item>
            <ion-input
              v-model="otp"
              label="2-step verification code (if the account uses it)"
              label-placement="stacked"
              inputmode="numeric"
              autocomplete="off"
              data-testid="setup-otp"
            />
          </ion-item>
        </ion-list>

        <ion-list inset>
          <ion-list-header><ion-note>SynoDL admin account</ion-note></ion-list-header>
          <ion-item>
            <ion-input
              v-model="adminUsername"
              label="Admin username"
              label-placement="stacked"
              autocapitalize="off"
              autocomplete="off"
              data-testid="setup-admin-username"
            />
          </ion-item>
          <ion-item>
            <ion-input
              v-model="adminPassword"
              label="Admin password (min 8 characters)"
              label-placement="stacked"
              type="password"
              autocomplete="new-password"
              data-testid="setup-admin-password"
            />
          </ion-item>
        </ion-list>

        <ion-list inset>
          <ion-list-header><ion-note>Optional</ion-note></ion-list-header>
          <ion-item>
            <ion-input
              v-model="publicUrl"
              label="Public URL"
              label-placement="stacked"
              placeholder="https://synodl.example.com"
              autocapitalize="off"
              data-testid="setup-public-url"
            />
          </ion-item>
        </ion-list>

        <ion-note v-if="error" color="danger" class="error" data-testid="setup-error">
          {{ error }}
        </ion-note>

        <ion-button
          expand="block"
          :disabled="!canSubmit"
          data-testid="setup-submit"
          @click="submit"
        >
          <ion-spinner v-if="busy" name="crescent" />
          <template v-else>Finish setup</template>
        </ion-button>
      </div>
    </ion-content>
  </ion-page>
</template>

<style scoped>
.setup-wrap {
  max-width: 30rem;
  margin: 0 auto;
  padding-top: 6vh;
  padding-bottom: 4rem;
}
h1 {
  text-align: center;
  font-weight: 700;
}
.intro {
  text-align: center;
  color: var(--app-text-dim);
  margin-top: -0.25rem;
}
.error {
  display: block;
  margin: 0.5rem 1rem;
}
</style>
