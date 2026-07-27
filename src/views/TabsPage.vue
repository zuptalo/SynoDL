<script setup lang="ts">
import { IonIcon, IonLabel, IonPage, IonRouterOutlet, IonTabBar, IonTabButton, IonTabs } from '@ionic/vue';
import { downloadOutline, globeOutline, logoRss, optionsOutline, searchOutline } from 'ionicons/icons';
import NasReauthGate from '@/components/NasReauthGate.vue';
</script>

<template>
  <ion-page>
    <!-- Global: prompts an admin to re-enter a 2FA code when the NAS session expires. -->
    <NasReauthGate />
    <ion-tabs>
      <ion-router-outlet />
      <ion-tab-bar slot="bottom" :translucent="true">
        <ion-tab-button tab="tasks" href="/tabs/tasks" data-testid="tab-tasks">
          <ion-icon aria-hidden="true" :icon="downloadOutline" />
          <ion-label>Tasks</ion-label>
        </ion-tab-button>
        <ion-tab-button tab="search" href="/tabs/search" data-testid="tab-search">
          <ion-icon aria-hidden="true" :icon="searchOutline" />
          <ion-label>Search</ion-label>
        </ion-tab-button>
        <ion-tab-button tab="browser" href="/tabs/browser" data-testid="tab-browser">
          <ion-icon aria-hidden="true" :icon="globeOutline" />
          <ion-label>Browser</ion-label>
        </ion-tab-button>
        <ion-tab-button tab="rss" href="/tabs/rss" data-testid="tab-rss">
          <ion-icon aria-hidden="true" :icon="logoRss" />
          <ion-label>RSS</ion-label>
        </ion-tab-button>
        <ion-tab-button tab="settings" href="/tabs/settings" data-testid="tab-settings">
          <ion-icon aria-hidden="true" :icon="optionsOutline" />
          <ion-label>Settings</ion-label>
        </ion-tab-button>
      </ion-tab-bar>
    </ion-tabs>
  </ion-page>
</template>
