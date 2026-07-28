<script setup lang="ts">
/**
 * Dive-in wrapper for user management (spec 1002 follow-up): the admin user list
 * + add-user form live behind a Settings row instead of inline, so Settings
 * stays scannable. The list logic itself stays in UserAdmin.
 */
import {
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import UserAdmin from '@/components/UserAdmin.vue';
import { IonModal } from '@ionic/vue';

defineProps<{ isOpen: boolean }>();
defineEmits<{ (e: 'dismiss'): void }>();
</script>

<template>
  <ion-modal :is-open="isOpen" @did-dismiss="$emit('dismiss')">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>User management</ion-title>
        <ion-buttons slot="end">
          <ion-button data-testid="users-close" @click="$emit('dismiss')">Done</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content :fullscreen="true" class="settings-cards">
      <UserAdmin data-testid="settings-useradmin" />
    </ion-content>
  </ion-modal>
</template>
