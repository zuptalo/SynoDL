<script setup lang="ts">
import { IonIcon, IonLabel, IonPage, IonRouterOutlet, IonTabBar, IonTabButton, IonTabs } from '@ionic/vue';
import { downloadOutline, globeOutline, logoRss, optionsOutline, searchOutline } from 'ionicons/icons';
</script>

<template>
  <ion-page>
    <ion-tabs>
      <ion-router-outlet />
      <ion-tab-bar slot="bottom">
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
