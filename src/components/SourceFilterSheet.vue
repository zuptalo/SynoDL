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

const props = defineProps<{ isOpen: boolean; filters: SourceSearchFilters; sort: string }>();
const emit = defineEmits<{
  (e: 'dismiss'): void;
  (e: 'apply', filters: SourceSearchFilters, sort: string): void;
}>();

// Option lists mirror the provider's advanced_search_parametres facets. Type,
// genre, language, country and score are sent as the provider's codes; type
// names are mapped to numeric codes server-side.
const sorts = [
  { value: 'year', label: 'Release year (newest)' },
  { value: 'date', label: 'Recently added' },
  { value: 'favorite', label: 'Most popular' },
];
const types = [
  { value: '', label: 'All types' },
  { value: 'movie', label: 'Movies' },
  { value: 'series', label: 'Series' },
  { value: 'anime', label: 'Anime' },
];
const scores = [
  { value: '', label: 'Any rating' },
  { value: '9', label: '9.0+' },
  { value: '8.5', label: '8.5+' },
  { value: '8', label: '8.0+' },
  { value: '7.5', label: '7.5+' },
  { value: '7', label: '7.0+' },
  { value: '6', label: '6.0+' },
  { value: '5', label: '5.0+' },
];
const qualities = [
  '4K',
  'Remux',
  'BluRay Full HD',
  'BluRay',
  'WEB-DL',
  'WEBRip',
  'HDTV',
  'DVDRip',
  'HDRip',
  'CAM',
];
const genres: { value: string; label: string }[] = [
  { value: '3355', label: 'Action' },
  { value: '3356', label: 'Adventure' },
  { value: '3357', label: 'Animation' },
  { value: '3358', label: 'Biography' },
  { value: '3359', label: 'Comedy' },
  { value: '3360', label: 'Crime' },
  { value: '3361', label: 'Documentary' },
  { value: '3362', label: 'Drama' },
  { value: '3363', label: 'Family' },
  { value: '3364', label: 'Fantasy' },
  { value: '3366', label: 'History' },
  { value: '3367', label: 'Horror' },
  { value: '3368', label: 'Music' },
  { value: '3370', label: 'Mystery' },
  { value: '3372', label: 'Romance' },
  { value: '3373', label: 'Sci-Fi' },
  { value: '3374', label: 'Short' },
  { value: '3375', label: 'Sport' },
  { value: '3377', label: 'Superhero' },
  { value: '3378', label: 'Thriller' },
  { value: '3379', label: 'War' },
  { value: '3380', label: 'Western' },
];
const languages = [
  { value: 'en', label: 'English' },
  { value: 'ja', label: 'Japanese' },
  { value: 'fr', label: 'French' },
  { value: 'ko', label: 'Korean' },
  { value: 'it', label: 'Italian' },
  { value: 'es', label: 'Spanish' },
  { value: 'hi', label: 'Hindi' },
  { value: 'de', label: 'German' },
  { value: 'zh', label: 'Chinese' },
  { value: 'ru', label: 'Russian' },
  { value: 'ar', label: 'Arabic' },
  { value: 'fa', label: 'Persian' },
];
const countries = [
  { value: 'US', label: 'United States' },
  { value: 'GB', label: 'United Kingdom' },
  { value: 'FR', label: 'France' },
  { value: 'CA', label: 'Canada' },
  { value: 'JP', label: 'Japan' },
  { value: 'IT', label: 'Italy' },
  { value: 'DE', label: 'Germany' },
  { value: 'IN', label: 'India' },
  { value: 'KR', label: 'South Korea' },
  { value: 'ES', label: 'Spain' },
  { value: 'AU', label: 'Australia' },
  { value: 'CN', label: 'China' },
  { value: 'IR', label: 'Iran' },
  { value: 'TR', label: 'Turkey' },
];

// Local working copy so Cancel discards edits. Genre is single-select here (sent
// to the API as a one-element array).
const type = ref('');
const genre = ref('');
const quality = ref('');
const language = ref('');
const country = ref('');
const score = ref('');
const localSort = ref('year');

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
    localSort.value = props.sort || 'year';
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
  emit('apply', f, localSort.value);
  emit('dismiss');
}

function clear(): void {
  type.value = '';
  genre.value = '';
  quality.value = '';
  language.value = '';
  country.value = '';
  score.value = '';
  localSort.value = 'year';
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
        <ion-title>Filters &amp; sort</ion-title>
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
          <ion-select v-model="localSort" label="Sort by" interface="popover">
            <ion-select-option v-for="s in sorts" :key="s.value" :value="s.value">
              {{ s.label }}
            </ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item>
          <ion-select v-model="type" label="Type" interface="popover">
            <ion-select-option v-for="t in types" :key="t.value" :value="t.value">
              {{ t.label }}
            </ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item>
          <ion-select v-model="genre" label="Genre" interface="alert" placeholder="Any">
            <ion-select-option value="">Any genre</ion-select-option>
            <ion-select-option v-for="g in genres" :key="g.value" :value="g.value">
              {{ g.label }}
            </ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item>
          <ion-select v-model="quality" label="Quality" interface="alert" placeholder="Any">
            <ion-select-option value="">Any quality</ion-select-option>
            <ion-select-option v-for="q in qualities" :key="q" :value="q">{{ q }}</ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item>
          <ion-select v-model="score" label="Min rating" interface="popover">
            <ion-select-option v-for="s in scores" :key="s.value" :value="s.value">
              {{ s.label }}
            </ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item>
          <ion-select v-model="language" label="Language" interface="alert" placeholder="Any">
            <ion-select-option value="">Any language</ion-select-option>
            <ion-select-option v-for="l in languages" :key="l.value" :value="l.value">
              {{ l.label }}
            </ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item>
          <ion-select v-model="country" label="Country" interface="alert" placeholder="Any">
            <ion-select-option value="">Any country</ion-select-option>
            <ion-select-option v-for="c in countries" :key="c.value" :value="c.value">
              {{ c.label }}
            </ion-select-option>
          </ion-select>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-modal>
</template>
