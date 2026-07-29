<script setup lang="ts">
import { ref, watch } from 'vue';
import {
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonItem,
  IonList,
  IonModal,
  IonSelect,
  IonSelectOption,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import type { SourceSearchFilters } from '@/services/api';
import { useSourceCatalog } from '@/composables/useSourceCatalog';

// Filter options track the provider's live facets (falling back to built-in
// lists), so the sheet always offers what the source currently supports.
const { filterOptions } = useSourceCatalog();

const props = defineProps<{ isOpen: boolean; filters: SourceSearchFilters }>();
const emit = defineEmits<{
  (e: 'dismiss'): void;
  (e: 'apply', filters: SourceSearchFilters): void;
}>();

// Local working copy so Cancel discards edits. Genre is single-select here (sent
// to the API as a one-element array).
const type = ref('');
const genre = ref('');
const quality = ref('');
const language = ref('');
const country = ref('');
const score = ref('');

watch(
  () => props.isOpen,
  (open) => {
    if (!open) return;
    const f = props.filters;
    type.value = f.type ?? '';
    genre.value = f.genre?.[0] ?? '';
    quality.value = f.quality ?? '';
    language.value = f.language ?? '';
    country.value = f.country ?? '';
    score.value = f.score ?? '';
  },
);

function apply(): void {
  const f: SourceSearchFilters = {};
  if (type.value) f.type = type.value;
  if (genre.value) f.genre = [genre.value];
  if (quality.value) f.quality = quality.value;
  if (language.value) f.language = language.value;
  if (country.value) f.country = country.value;
  if (score.value) f.score = score.value;
  emit('apply', f);
  emit('dismiss');
}

function clear(): void {
  type.value = '';
  genre.value = '';
  quality.value = '';
  language.value = '';
  country.value = '';
  score.value = '';
}
</script>

<template>
  <ion-modal
    :is-open="isOpen"
    :initial-breakpoint="0.75"
    :breakpoints="[0, 0.75, 1]"
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
          <ion-select v-model="type" label="Type" interface="popover">
            <ion-select-option v-for="t in filterOptions.types" :key="t.value" :value="t.value">
              {{ t.label }}
            </ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item>
          <ion-select v-model="genre" label="Genre" interface="alert" placeholder="Any">
            <ion-select-option v-for="g in filterOptions.genres" :key="g.value" :value="g.value">
              {{ g.label }}
            </ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item>
          <ion-select v-model="quality" label="Quality" interface="alert" placeholder="Any">
            <ion-select-option v-for="q in filterOptions.qualities" :key="q.value" :value="q.value">
              {{ q.label }}
            </ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item>
          <ion-select v-model="score" label="Min rating" interface="popover">
            <ion-select-option v-for="s in filterOptions.scores" :key="s.value" :value="s.value">
              {{ s.label }}
            </ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item>
          <ion-select v-model="language" label="Language" interface="alert" placeholder="Any">
            <ion-select-option v-for="l in filterOptions.languages" :key="l.value" :value="l.value">
              {{ l.label }}
            </ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item>
          <ion-select v-model="country" label="Country" interface="alert" placeholder="Any">
            <ion-select-option v-for="c in filterOptions.countries" :key="c.value" :value="c.value">
              {{ c.label }}
            </ion-select-option>
          </ion-select>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-modal>
</template>
