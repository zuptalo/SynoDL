<script setup lang="ts">
import { ref, watch } from 'vue';
import {
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonItem,
  IonLabel,
  IonList,
  IonModal,
  IonSelect,
  IonSelectOption,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import type { SourceSearchFilters } from '@/services/api';

const props = defineProps<{ isOpen: boolean; filters: SourceSearchFilters }>();
const emit = defineEmits<{
  (e: 'dismiss'): void;
  (e: 'apply', filters: SourceSearchFilters): void;
}>();

// Local working copy so Cancel discards edits.
const local = ref<SourceSearchFilters>({});
watch(
  () => props.isOpen,
  (open) => {
    if (open) local.value = { ...props.filters };
  },
);

const types = [
  { value: '', label: 'Any type' },
  { value: 'movie', label: 'Movies' },
  { value: 'series', label: 'Series' },
  { value: 'anime', label: 'Anime' },
];
const qualities = ['', '4K', '2160p', '1080p', '720p', '480p'];

function apply(): void {
  emit('apply', { ...local.value });
  emit('dismiss');
}
function clear(): void {
  local.value = {};
}
</script>

<template>
  <ion-modal
    :is-open="isOpen"
    :initial-breakpoint="0.6"
    :breakpoints="[0, 0.6, 0.9]"
    @didDismiss="emit('dismiss')"
  >
    <ion-header>
      <ion-toolbar>
        <ion-title>Filters</ion-title>
        <ion-buttons slot="start">
          <ion-button @click="clear">Clear</ion-button>
        </ion-buttons>
        <ion-buttons slot="end">
          <ion-button :strong="true" @click="apply">Apply</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content>
      <ion-list :inset="true">
        <ion-item>
          <ion-select
            v-model="local.type"
            label="Type"
            interface="popover"
            placeholder="Any type"
          >
            <ion-select-option v-for="t in types" :key="t.value" :value="t.value">
              {{ t.label }}
            </ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item>
          <ion-select
            v-model="local.quality"
            label="Quality"
            interface="popover"
            placeholder="Any quality"
          >
            <ion-select-option v-for="q in qualities" :key="q" :value="q">
              {{ q || 'Any quality' }}
            </ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item>
          <ion-label position="stacked">Language (ISO code)</ion-label>
          <ion-select
            v-model="local.language"
            interface="popover"
            placeholder="Any"
          >
            <ion-select-option value="">Any</ion-select-option>
            <ion-select-option value="en">English (en)</ion-select-option>
            <ion-select-option value="fa">Persian (fa)</ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item>
          <ion-select
            v-model="local.country"
            label="Country"
            interface="popover"
            placeholder="Any"
          >
            <ion-select-option value="">Any</ion-select-option>
            <ion-select-option value="US">United States</ion-select-option>
            <ion-select-option value="GB">United Kingdom</ion-select-option>
          </ion-select>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-modal>
</template>
