<script setup lang="ts">
import { IonApp, IonRouterOutlet, IonToast } from '@ionic/vue';
import { useAppUpdate } from '@/composables/useAppUpdate';

// Prompt-based updates: the toast names the waiting version; applying is the
// user's call, never automatic (constitution Principle V).
const { updateAvailable, applyUpdate } = useAppUpdate();
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
