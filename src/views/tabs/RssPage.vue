<script setup lang="ts">
import { IonContent, IonHeader, IonPage, IonTitle, IonToolbar } from '@ionic/vue';
</script>

<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>RSS</ion-title>
      </ion-toolbar>
    </ion-header>
    <ion-content :fullscreen="true">
      <div class="placeholder">
        <p>Follow RSS feeds and download new items automatically.</p>
        <p class="soon">Coming soon — see the roadmap.</p>
      </div>
    </ion-content>
  </ion-page>
</template>

<style scoped>
.placeholder {
  padding-top: 30vh;
  text-align: center;
  color: var(--app-text-dim);
}
.soon {
  font-size: 0.8rem;
}
</style>
