<script setup lang="ts">
/**
 * Admin-only editor for the stored NAS connection (spec 1002). Loads the
 * non-secret config on open, lets the admin change host/port/TLS/account/
 * password/OTP, TEST the connection before committing, and SAVE (the server
 * re-verifies and rolls back on failure). The password field is blank on load
 * and a blank value keeps the stored secret — it's write-only end to end.
 */
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
  IonTitle,
  IonToggle,
  IonToolbar,
} from '@ionic/vue';
import { ref } from 'vue';
import { api, type NasConnInput } from '@/services/api';
import { messageForError } from '@/services/syno-errors';

const emit = defineEmits<{ (e: 'dismiss'): void; (e: 'saved'): void }>();
defineProps<{ isOpen: boolean }>();

const loading = ref(false);
const busy = ref(false);
const testState = ref<'idle' | 'ok' | 'error'>('idle');
const message = ref('');

const publicUrl = ref('');
const nasAddress = ref('');
const nasPort = ref<number>(5001);
const nasTlsVerify = ref(true);
const nasAccount = ref('');
const nasPassword = ref('');
const otp = ref('');
const uses2FA = ref(false);

async function onOpen(): Promise<void> {
  loading.value = true;
  message.value = '';
  testState.value = 'idle';
  try {
    const c = await api.getNasConfig();
    publicUrl.value = c.publicUrl;
    nasAddress.value = c.nasAddress;
    nasPort.value = c.nasPort;
    nasTlsVerify.value = c.nasTlsVerify;
    nasAccount.value = c.nasAccount;
    nasPassword.value = ''; // never prefilled; blank keeps the stored secret
    uses2FA.value = c.nasUses2FA;
    otp.value = '';
  } catch (e) {
    message.value = describe(e);
  } finally {
    loading.value = false;
  }
}

function payload(): NasConnInput {
  const p: NasConnInput = {
    publicUrl: publicUrl.value.trim(),
    nasAddress: nasAddress.value.trim(),
    nasPort: Number(nasPort.value) || 0,
    nasTlsVerify: nasTlsVerify.value,
    nasAccount: nasAccount.value.trim(),
  };
  if (nasPassword.value) p.nasPassword = nasPassword.value;
  if (otp.value.trim()) p.otp = otp.value.trim();
  return p;
}

function describe(e: unknown): string {
  return messageForError(e);
}

async function onTest(): Promise<void> {
  busy.value = true;
  message.value = '';
  testState.value = 'idle';
  try {
    await api.testNasConnection(payload());
    testState.value = 'ok';
    message.value = 'Connection succeeded.';
  } catch (e) {
    testState.value = 'error';
    message.value = describe(e);
  } finally {
    busy.value = false;
  }
}

async function onSave(): Promise<void> {
  busy.value = true;
  message.value = '';
  try {
    await api.updateNasConfig(payload());
    emit('saved');
    emit('dismiss');
  } catch (e) {
    testState.value = 'error';
    message.value = describe(e);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <ion-modal :is-open="isOpen" @will-present="onOpen" @did-dismiss="$emit('dismiss')">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>NAS connection</ion-title>
        <ion-buttons slot="start">
          <ion-button data-testid="nas-cancel" @click="$emit('dismiss')">Cancel</ion-button>
        </ion-buttons>
        <ion-buttons slot="end">
          <ion-button :disabled="busy || loading" data-testid="nas-save" @click="onSave">Save</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content :fullscreen="true" class="ion-padding">
      <div v-if="loading" class="center"><ion-spinner name="crescent" /></div>
      <template v-else>
        <ion-list inset>
          <ion-list-header>Public address</ion-list-header>
          <ion-item>
            <ion-input
              label="Public URL"
              label-placement="stacked"
              placeholder="https://synodl.example.com"
              :value="publicUrl"
              data-testid="nas-public-url"
              @ionInput="publicUrl = String($event.target.value ?? '')"
            />
          </ion-item>
        </ion-list>

        <ion-list inset>
          <ion-list-header>NAS</ion-list-header>
          <ion-item>
            <ion-input
              label="Address"
              label-placement="stacked"
              placeholder="nas.local"
              :value="nasAddress"
              data-testid="nas-address"
              @ionInput="nasAddress = String($event.target.value ?? '')"
            />
          </ion-item>
          <ion-item>
            <ion-input
              label="Port"
              label-placement="stacked"
              type="number"
              inputmode="numeric"
              :value="nasPort"
              data-testid="nas-port"
              @ionInput="nasPort = Number($event.target.value ?? 0)"
            />
          </ion-item>
          <ion-item>
            <ion-toggle
              :checked="nasTlsVerify"
              data-testid="nas-tls"
              @ionChange="nasTlsVerify = $event.detail.checked"
            >
              Verify TLS certificate
            </ion-toggle>
          </ion-item>
        </ion-list>

        <ion-list inset>
          <ion-list-header>Credentials</ion-list-header>
          <ion-item>
            <ion-input
              label="Account"
              label-placement="stacked"
              :value="nasAccount"
              data-testid="nas-account"
              @ionInput="nasAccount = String($event.target.value ?? '')"
            />
          </ion-item>
          <ion-item>
            <ion-input
              label="Password"
              label-placement="stacked"
              type="password"
              placeholder="Leave blank to keep current"
              :value="nasPassword"
              data-testid="nas-password"
              @ionInput="nasPassword = String($event.target.value ?? '')"
            />
          </ion-item>
          <ion-item v-if="uses2FA || otp">
            <ion-input
              label="2-step code (OTP)"
              label-placement="stacked"
              inputmode="numeric"
              placeholder="Required to re-verify a 2FA account"
              :value="otp"
              data-testid="nas-otp"
              @ionInput="otp = String($event.target.value ?? '')"
            />
          </ion-item>
        </ion-list>

        <div class="actions">
          <ion-button expand="block" fill="outline" :disabled="busy" data-testid="nas-test" @click="onTest">
            <ion-spinner v-if="busy" name="crescent" />
            <span v-else>Test connection</span>
          </ion-button>
          <ion-note
            v-if="message"
            :color="testState === 'ok' ? 'success' : 'danger'"
            data-testid="nas-message"
            >{{ message }}</ion-note
          >
        </div>
      </template>
    </ion-content>
  </ion-modal>
</template>

<style scoped>
.center {
  display: flex;
  justify-content: center;
  padding-top: 30vh;
}
.actions {
  margin: 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  align-items: center;
}
</style>
