<script setup lang="ts">
import {
  IonButton,
  IonContent,
  IonHeader,
  IonItem,
  IonLabel,
  IonList,
  IonNote,
  IonPage,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { api } from '@/services/api';
import { useSession } from '@/composables/useSession';

const router = useRouter();
const { account, logout } = useSession();
const nasHost = ref('');
// Template scope can't see compile-time globals; re-expose the define.
const version = __APP_VERSION__;

onMounted(async () => {
  try {
    nasHost.value = (await api.config()).nasHost;
  } catch {
    /* offline — the rows just stay empty */
  }
});

async function onLogout(): Promise<void> {
  await logout();
  await router.replace('/login');
}
</script>

<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-title>Settings</ion-title>
      </ion-toolbar>
    </ion-header>
    <ion-content>
      <ion-list inset>
        <ion-item>
          <ion-label>Host</ion-label>
          <ion-note slot="end" color="primary" data-testid="settings-host">{{ nasHost }}</ion-note>
        </ion-item>
        <ion-item>
          <ion-label>Account</ion-label>
          <ion-note slot="end" color="primary" data-testid="settings-account">{{ account }}</ion-note>
        </ion-item>
      </ion-list>

      <div class="logout">
        <ion-button expand="block" color="danger" fill="outline" data-testid="settings-logout" @click="onLogout">
          Logout
        </ion-button>
      </div>

      <p class="version" data-testid="settings-version">v{{ version }}</p>
    </ion-content>
  </ion-page>
</template>

<style scoped>
.logout {
  margin: 2rem 1rem 0;
}
.version {
  text-align: center;
  color: var(--app-text-dim);
  font-size: 0.8rem;
  margin-top: 2rem;
}
</style>
