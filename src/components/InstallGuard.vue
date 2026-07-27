<script setup lang="ts">
/**
 * Full-screen install gate (spec 1008). Overlays the always-mounted router
 * outlet as an opaque page so a plain browser tab can't be used — you must add
 * SynoDL to the Home Screen (install it) before you can sign in. localhost is
 * exempt (see useInstallGuard) so dev/e2e aren't blocked.
 */
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
  IonText,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { downloadOutline, warningOutline } from 'ionicons/icons';
import { computed } from 'vue';
import { promptInstall, useInstallGuard } from '@/composables/useInstallGuard';

const { mustInstall, platform, canPrompt, installUnavailable, firefoxAndroid } = useInstallGuard();

const steps = computed<string[]>(() => {
  switch (platform.value) {
    case 'ios':
      return [
        'Tap the Share button in Safari.',
        'Choose "Add to Home Screen".',
        'Open SynoDL from your Home Screen.',
      ];
    case 'android':
      return [
        'Tap the ⋮ menu in your browser.',
        'Choose "Install app" (or "Add to Home screen").',
        'Open SynoDL from your Home Screen.',
      ];
    default:
      return [
        'Click the install icon in your browser\'s address bar.',
        'Confirm to install SynoDL.',
        'Open SynoDL from your apps.',
      ];
  }
});
</script>

<template>
  <ion-page v-if="mustInstall">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>Install SynoDL</ion-title>
      </ion-toolbar>
    </ion-header>
    <ion-content :fullscreen="true">
      <div class="wrap">
        <div class="brand">
          <div class="tile">
            <ion-icon :icon="downloadOutline" />
          </div>
          <ion-text><h1>SynoDL</h1></ion-text>
          <ion-text color="medium">
            <p>For reliable notifications, add SynoDL to your Home Screen to continue.</p>
          </ion-text>
        </div>

        <div v-if="platform === 'android' && installUnavailable" class="callout">
          <ion-icon :icon="warningOutline" />
          <span>
            You've opened SynoDL inside another app's browser, which can't install apps.
            Open it in Chrome (or your browser app), then install from there.
          </span>
        </div>
        <div v-else-if="platform === 'android' && firefoxAndroid" class="callout">
          <ion-icon :icon="warningOutline" />
          <span>
            Firefox on Android can't install SynoDL as an app. Open the site in Chrome or
            Samsung Internet, then follow the steps below.
          </span>
        </div>

        <div v-if="canPrompt" class="install-btn">
          <ion-button expand="block" shape="round" data-testid="install-button" @click="promptInstall">
            <ion-icon slot="start" :icon="downloadOutline" />
            Install SynoDL
          </ion-button>
        </div>

        <ion-list :inset="true">
          <ion-list-header><ion-label>How to install</ion-label></ion-list-header>
          <ion-item v-for="(step, i) in steps" :key="i" lines="none">
            <ion-note slot="start" class="num">{{ i + 1 }}</ion-note>
            <ion-label class="ion-text-wrap">{{ step }}</ion-label>
          </ion-item>
        </ion-list>

        <div class="footer">
          <ion-text color="medium"><p>Already added it? Open SynoDL from your Home Screen.</p></ion-text>
        </div>
      </div>
    </ion-content>
  </ion-page>
</template>

<style scoped>
.wrap {
  max-width: 480px;
  margin: 0 auto;
}
.brand {
  text-align: center;
  padding: 1.5rem 1rem 0.5rem;
}
.tile {
  width: 76px;
  height: 76px;
  margin: 0 auto 1rem;
  border-radius: 22px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--ion-color-primary);
  box-shadow: 0 12px 32px rgba(16, 185, 129, 0.45);
}
.tile ion-icon {
  font-size: 42px;
  color: #fff;
}
.callout {
  display: flex;
  gap: 0.6rem;
  align-items: flex-start;
  margin: 0.5rem 1rem;
  padding: 0.75rem;
  border-radius: 12px;
  background: var(--app-card);
  font-size: 0.85rem;
}
.callout ion-icon {
  font-size: 20px;
  color: var(--app-status-waiting);
  flex-shrink: 0;
}
.install-btn {
  padding: 0.5rem 1rem;
}
.num {
  min-width: 24px;
  text-align: center;
  font-weight: 700;
  color: var(--ion-color-primary);
}
.footer {
  text-align: center;
  padding: 1rem;
}
</style>
