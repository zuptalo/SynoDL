<script setup lang="ts">
import {
  IonButton,
  IonContent,
  IonHeader,
  IonIcon,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonNote,
  IonPage,
  IonTitle,
  IonToggle,
  IonToolbar,
} from '@ionic/vue';
import { chevronForward } from 'ionicons/icons';
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { api } from '@/services/api';
import { useSession } from '@/composables/useSession';
import { useTheme } from '@/composables/useTheme';
import UserManagementModal from '@/components/UserManagementModal.vue';
import PushOptIn from '@/components/PushOptIn.vue';
import NasConnectionModal from '@/components/NasConnectionModal.vue';
import ChangePasswordModal from '@/components/ChangePasswordModal.vue';
import SourceProviderAdmin from '@/components/SourceProviderAdmin.vue';

const router = useRouter();
const { account, logout, isAdmin, mode, user } = useSession();
const { theme, setTheme } = useTheme();
const nasHost = ref('');

// Template scope can't see compile-time globals; re-expose the define.
const version = __APP_VERSION__;

const stateful = computed(() => mode.value === 'stateful');
const darkMode = computed({
  get: () => theme.value === 'dark',
  set: (on: boolean) => setTheme(on ? 'dark' : 'light'),
});

const nasOpen = ref(false);
const pwOpen = ref(false);
const usersOpen = ref(false);
const sourceOpen = ref(false);

async function loadHost(): Promise<void> {
  try {
    nasHost.value = (await api.config()).nasHost;
  } catch {
    /* offline — the row just stays empty */
  }
}
onMounted(loadHost);

async function onLogout(): Promise<void> {
  await logout();
  await router.replace('/login');
}
</script>

<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>Settings</ion-title>
      </ion-toolbar>
    </ion-header>
    <ion-content :fullscreen="true">
      <!-- Account -->
      <ion-list inset>
        <ion-list-header>Account</ion-list-header>
        <ion-item>
          <ion-label>Signed in as</ion-label>
          <ion-note slot="end" color="primary" data-testid="settings-account">{{ account }}</ion-note>
        </ion-item>
        <ion-item
          v-if="stateful && user"
          button
          :detail="false"
          data-testid="settings-change-password"
          @click="pwOpen = true"
        >
          <ion-label>Change password</ion-label>
          <ion-icon slot="end" :icon="chevronForward" color="medium" />
        </ion-item>
      </ion-list>

      <!-- Appearance -->
      <ion-list inset>
        <ion-list-header>Appearance</ion-list-header>
        <ion-item>
          <ion-toggle v-model="darkMode" data-testid="settings-dark-toggle">Dark mode</ion-toggle>
        </ion-item>
      </ion-list>

      <!-- NAS connection (admin, stateful) — dive-in editor -->
      <ion-list v-if="stateful && isAdmin" inset>
        <ion-list-header>NAS connection</ion-list-header>
        <ion-item button :detail="false" data-testid="settings-nas-connection" @click="nasOpen = true">
          <ion-label>
            <h2>Connection</h2>
            <p>{{ nasHost || 'Configure the NAS this app talks to' }}</p>
          </ion-label>
          <ion-icon slot="end" :icon="chevronForward" color="medium" />
        </ion-item>
      </ion-list>

      <!-- Legacy (stateless) mode just shows the host read-only. -->
      <ion-list v-else-if="!stateful" inset>
        <ion-list-header>NAS</ion-list-header>
        <ion-item>
          <ion-label>Host</ion-label>
          <ion-note slot="end" color="primary" data-testid="settings-host">{{ nasHost }}</ion-note>
        </ion-item>
      </ion-list>

      <!-- Notifications (stateful only). -->
      <PushOptIn v-if="stateful" />

      <!-- Admin-only: user management behind its own dive-in section. -->
      <ion-list v-if="isAdmin" inset>
        <ion-list-header>Users</ion-list-header>
        <ion-item button :detail="false" data-testid="settings-users" @click="usersOpen = true">
          <ion-label>Manage users</ion-label>
          <ion-icon slot="end" :icon="chevronForward" color="medium" />
        </ion-item>
      </ion-list>

      <!-- Admin-only: configure the external download source (spec 0005). -->
      <ion-list v-if="stateful && isAdmin" inset>
        <ion-list-header>Download source</ion-list-header>
        <ion-item button :detail="false" data-testid="settings-source" @click="sourceOpen = true">
          <ion-label>Configure download source</ion-label>
          <ion-icon slot="end" :icon="chevronForward" color="medium" />
        </ion-item>
      </ion-list>

      <div class="logout">
        <ion-button expand="block" color="danger" fill="outline" data-testid="settings-logout" @click="onLogout">
          Logout
        </ion-button>
      </div>

      <p class="version" data-testid="settings-version">v{{ version }}</p>

      <UserManagementModal v-if="isAdmin" :is-open="usersOpen" @dismiss="usersOpen = false" />
      <SourceProviderAdmin v-if="isAdmin" :is-open="sourceOpen" @dismiss="sourceOpen = false" />
      <NasConnectionModal :is-open="nasOpen" @dismiss="nasOpen = false" @saved="loadHost" />
      <ChangePasswordModal
        v-if="user"
        :is-open="pwOpen"
        :user-id="user.id"
        @dismiss="pwOpen = false"
      />
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
