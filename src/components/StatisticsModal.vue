<script setup lang="ts">
/**
 * Dive-in wrapper for the Statistics section (spec 0006), mirroring the
 * UserManagementModal pattern: a thin ion-modal + header that hosts the real
 * StatisticsView. Available to every signed-in user for their own stats; admins
 * additionally get the in-view user picker.
 */
import {
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonModal,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import StatisticsView from '@/components/StatisticsView.vue';

defineProps<{ isOpen: boolean; isAdmin: boolean }>();
defineEmits<{ (e: 'dismiss'): void }>();
</script>

<template>
  <ion-modal :is-open="isOpen" @did-dismiss="$emit('dismiss')">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>Download statistics</ion-title>
        <ion-buttons slot="end">
          <ion-button data-testid="stats-close" @click="$emit('dismiss')">Done</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content :fullscreen="true" class="settings-cards">
      <StatisticsView v-if="isOpen" :is-admin="isAdmin" data-testid="settings-statisticsview" />
    </ion-content>
  </ion-modal>
</template>
