<script setup lang="ts">
import { ref, watch } from 'vue';
import {
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonInput,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonModal,
  IonSelect,
  IonSelectOption,
  IonTitle,
  IonToggle,
  IonToolbar,
} from '@ionic/vue';
import type { SourceSearchFilters } from '@/services/api';
import { useSourceCatalog } from '@/composables/useSourceCatalog';

// Filter options track the provider's live facets (falling back to built-in
// lists), so the sheet always offers what the source currently supports.
const { filterOptions, yearBounds } = useSourceCatalog();

const props = defineProps<{ isOpen: boolean; filters: SourceSearchFilters }>();
const emit = defineEmits<{
  (e: 'dismiss'): void;
  (e: 'apply', filters: SourceSearchFilters): void;
}>();

// Local working copy so Cancel discards edits. Genre is single-select here (sent
// to the API as a one-element array). Toggles are booleans locally, mapped to the
// provider's "true" flag on apply.
const type = ref('');
const genre = ref('');
const quality = ref('');
const language = ref('');
const country = ref('');
const score = ref('');
const channel = ref('');
const encoder = ref('');
const age = ref('');
const cast = ref('');
const director = ref('');
const creator = ref('');
const yearFrom = ref('');
const yearTo = ref('');
const x265 = ref(false);
const threeD = ref(false);
const stream = ref(false);

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
    channel.value = f.channel ?? '';
    encoder.value = f.encoder ?? '';
    age.value = f.age ?? '';
    cast.value = f.cast ?? '';
    director.value = f.director ?? '';
    creator.value = f.creator ?? '';
    yearFrom.value = f.yearFrom ?? '';
    yearTo.value = f.yearTo ?? '';
    x265.value = f.x265 === 'true';
    threeD.value = f.threeD === 'true';
    stream.value = f.stream === 'true';
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
  if (channel.value) f.channel = channel.value;
  if (encoder.value) f.encoder = encoder.value;
  if (age.value) f.age = age.value;
  const castV = cast.value.trim();
  if (castV) f.cast = castV;
  const directorV = director.value.trim();
  if (directorV) f.director = directorV;
  const creatorV = creator.value.trim();
  if (creatorV) f.creator = creatorV;
  if (String(yearFrom.value).trim()) f.yearFrom = String(yearFrom.value).trim();
  if (String(yearTo.value).trim()) f.yearTo = String(yearTo.value).trim();
  if (x265.value) f.x265 = 'true';
  if (threeD.value) f.threeD = 'true';
  if (stream.value) f.stream = 'true';
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
  channel.value = '';
  encoder.value = '';
  age.value = '';
  cast.value = '';
  director.value = '';
  creator.value = '';
  yearFrom.value = '';
  yearTo.value = '';
  x265.value = false;
  threeD.value = false;
  stream.value = false;
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

      <ion-list :inset="true">
        <ion-list-header><ion-label>More filters</ion-label></ion-list-header>
        <ion-item v-if="filterOptions.channels.length > 1">
          <ion-select v-model="channel" label="Channel" interface="alert" placeholder="Any">
            <ion-select-option v-for="c in filterOptions.channels" :key="c.value" :value="c.value">
              {{ c.label }}
            </ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item v-if="filterOptions.encoders.length > 1">
          <ion-select v-model="encoder" label="Encoder" interface="alert" placeholder="Any">
            <ion-select-option v-for="e in filterOptions.encoders" :key="e.value" :value="e.value">
              {{ e.label }}
            </ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item v-if="filterOptions.ages.length > 1">
          <ion-select v-model="age" label="Content rating" interface="alert" placeholder="Any">
            <ion-select-option v-for="a in filterOptions.ages" :key="a.value" :value="a.value">
              {{ a.label }}
            </ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item>
          <ion-input
            v-model="yearFrom"
            type="number"
            label="Year from"
            label-placement="fixed"
            :placeholder="String(yearBounds.min)"
            :min="yearBounds.min"
            :max="yearBounds.max"
            inputmode="numeric"
          />
        </ion-item>
        <ion-item>
          <ion-input
            v-model="yearTo"
            type="number"
            label="Year to"
            label-placement="fixed"
            :placeholder="String(yearBounds.max)"
            :min="yearBounds.min"
            :max="yearBounds.max"
            inputmode="numeric"
          />
        </ion-item>
        <ion-item>
          <ion-input v-model="cast" label="Cast" label-placement="fixed" placeholder="Actor name" />
        </ion-item>
        <ion-item>
          <ion-input v-model="director" label="Director" label-placement="fixed" placeholder="Name" />
        </ion-item>
        <ion-item>
          <ion-input v-model="creator" label="Creator" label-placement="fixed" placeholder="Name" />
        </ion-item>
        <ion-item>
          <ion-toggle v-model="x265">x265 / HEVC only</ion-toggle>
        </ion-item>
        <ion-item>
          <ion-toggle v-model="threeD">3D only</ion-toggle>
        </ion-item>
        <ion-item>
          <ion-toggle v-model="stream">Streamable only</ion-toggle>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-modal>
</template>
