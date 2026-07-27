<script setup lang="ts">
import {
  IonButton,
  IonContent,
  IonInput,
  IonItem,
  IonList,
  IonNote,
  IonPage,
  IonSpinner,
} from '@ionic/vue';
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { api, ApiError } from '@/services/api';
import { messageForError } from '@/services/syno-errors';
import { useSession } from '@/composables/useSession';

const router = useRouter();
const { login } = useSession();

const nasHost = ref('');
const account = ref('');
const password = ref('');
const otp = ref('');
const otpNeeded = ref(false);
const busy = ref(false);
const error = ref('');

onMounted(async () => {
  try {
    nasHost.value = (await api.config()).nasHost;
  } catch {
    // The proxy is unreachable — the connect attempt will say so properly.
  }
});

async function submit(): Promise<void> {
  busy.value = true;
  error.value = '';
  try {
    await login(account.value.trim(), password.value, otp.value.trim() || undefined);
    await router.replace('/tabs/tasks');
  } catch (e) {
    if (e instanceof ApiError && e.code === 'otp_required') otpNeeded.value = true;
    error.value = messageForError(e);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <ion-page>
    <ion-content class="ion-padding">
      <div class="login-wrap">
        <h1>SynoDL</h1>
        <p v-if="nasHost" class="host" data-testid="login-host">{{ nasHost }}</p>

        <ion-list inset>
          <ion-item>
            <ion-input
              v-model="account"
              label="Account"
              label-placement="stacked"
              autocomplete="username"
              autocapitalize="off"
              data-testid="login-account"
            />
          </ion-item>
          <ion-item>
            <ion-input
              v-model="password"
              label="Password"
              label-placement="stacked"
              type="password"
              autocomplete="current-password"
              data-testid="login-password"
              @keyup.enter="submit"
            />
          </ion-item>
          <ion-item v-if="otpNeeded">
            <ion-input
              v-model="otp"
              label="2-step verification code"
              label-placement="stacked"
              inputmode="numeric"
              data-testid="login-otp"
              @keyup.enter="submit"
            />
          </ion-item>
        </ion-list>

        <ion-note v-if="error" color="danger" class="error" data-testid="login-error">
          {{ error }}
        </ion-note>

        <ion-button
          expand="block"
          :disabled="busy || !account || !password"
          data-testid="login-submit"
          @click="submit"
        >
          <ion-spinner v-if="busy" name="crescent" />
          <template v-else>Connect</template>
        </ion-button>
      </div>
    </ion-content>
  </ion-page>
</template>

<style scoped>
.login-wrap {
  max-width: 26rem;
  margin: 0 auto;
  padding-top: 18vh;
}
h1 {
  text-align: center;
  font-weight: 700;
}
.host {
  text-align: center;
  color: var(--app-text-dim);
  margin-top: -0.25rem;
}
.error {
  display: block;
  margin: 0.5rem 1rem;
}
</style>
