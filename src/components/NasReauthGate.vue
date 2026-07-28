<script setup lang="ts">
/**
 * Global gate for the NAS 2FA re-auth flow. When any request comes back
 * 503 nas_reauth (the stored NAS session expired and the account uses 2FA, so it
 * can't be renewed unattended), this prompts an admin for a fresh code and
 * reconnects; a non-admin is told to ask an admin. Renders nothing itself.
 */
import { alertController } from '@ionic/vue';
import { onMounted, onUnmounted } from 'vue';
import { api, NAS_REAUTH_EVENT } from '@/services/api';
import { appToast } from '@/services/toast';
import { useSession } from '@/composables/useSession';

const { isAdmin } = useSession();
let showing = false;

async function handle(): Promise<void> {
  if (showing) return; // one prompt at a time, even if many requests fail at once
  showing = true;
  try {
    if (!isAdmin.value) {
      await appToast({
        message: 'The NAS connection expired — an admin needs to reconnect.',
        duration: 4000,
        color: 'warning',
      });
      return;
    }
    const alert = await alertController.create({
      header: 'Reconnect to your NAS',
      message: 'The NAS session expired. Enter a 2-step verification code to reconnect.',
      inputs: [{ name: 'otp', type: 'text', placeholder: '2-step code', attributes: { inputmode: 'numeric' } }],
      buttons: [
        { text: 'Later', role: 'cancel' },
        { text: 'Reconnect', role: 'confirm' },
      ],
      backdropDismiss: false,
    });
    await alert.present();
    const { role, data } = await alert.onDidDismiss();
    if (role !== 'confirm') return;
    try {
      await api.nasReauth((data?.values?.otp ?? '').trim());
      window.location.reload(); // re-fetch everything with the restored session
    } catch {
      await appToast({
        message: 'That code was not accepted. Please try again.',
        duration: 3000,
        color: 'danger',
      });
    }
  } finally {
    showing = false;
  }
}

function onEvent(): void {
  void handle();
}
onMounted(() => window.addEventListener(NAS_REAUTH_EVENT, onEvent));
onUnmounted(() => window.removeEventListener(NAS_REAUTH_EVENT, onEvent));
</script>

<template>
  <span style="display: none" aria-hidden="true" />
</template>
