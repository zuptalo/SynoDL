<script setup lang="ts">
import {
  IonButton,
  IonButtons,
  IonCheckbox,
  IonContent,
  IonHeader,
  IonInput,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonModal,
  IonRadio,
  IonRadioGroup,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { ref, watch } from 'vue';
import { ALL_STATUSES, type SortKey, type TaskFilterState } from '@/services/task-sort';

const props = defineProps<{ isOpen: boolean; filter: TaskFilterState }>();
const emit = defineEmits<{
  (e: 'apply', filter: TaskFilterState): void;
  (e: 'dismiss'): void;
}>();

// Sheet order mirrors the reference app's Filters screen.
const SORT_KEYS: Array<{ key: SortKey; label: string }> = [
  { key: 'createdAt', label: 'Creation date' },
  { key: 'status', label: 'Status' },
  { key: 'size', label: 'Size' },
  { key: 'peers', label: 'Peers' },
  { key: 'downloadSpeed', label: 'Download speed' },
  { key: 'uploadSpeed', label: 'Upload speed' },
  { key: 'name', label: 'Name' },
  { key: 'ratio', label: 'Share ratio' },
  { key: 'progress', label: 'Progress (ongoing tasks)' },
  { key: 'remaining', label: 'Remaining time (ongoing tasks)' },
];

const STATUS_LABELS: Record<string, string> = {
  finished: 'Finished',
  extracting: 'Extracting',
  finishing: 'Finishing',
  hash_checking: 'Hash checking',
  downloading: 'Downloading',
  paused: 'Paused',
  stopped: 'Stopped',
  waiting: 'Waiting',
  filehosting_waiting: 'File hosting waiting',
  moving: 'Moving',
  seeding: 'Seeding',
  error: 'Error',
};

// Draft edits live locally; only Apply commits them (US5 scenario 3).
const term = ref('');
const sortKey = ref<SortKey>('createdAt');
const ascending = ref(false);
const statuses = ref<Set<string>>(new Set());

watch(
  () => props.isOpen,
  (open) => {
    if (open) {
      term.value = props.filter.term;
      sortKey.value = props.filter.sortKey;
      ascending.value = props.filter.ascending;
      statuses.value = new Set(props.filter.statuses);
    }
  },
);

function toggleStatus(status: string, checked: boolean): void {
  const next = new Set(statuses.value);
  if (checked) next.add(status);
  else next.delete(status);
  statuses.value = next;
}

function apply(): void {
  emit('apply', {
    term: term.value,
    sortKey: sortKey.value,
    ascending: ascending.value,
    statuses: ALL_STATUSES.filter((s) => statuses.value.has(s)),
  });
}
</script>

<template>
  <ion-modal :is-open="isOpen" :initial-breakpoint="0.85" :breakpoints="[0, 0.85, 1]" @didDismiss="emit('dismiss')">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>Filters</ion-title>
        <ion-buttons slot="end">
          <ion-button data-testid="filter-apply" @click="apply">Apply</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content>
      <ion-list>
        <ion-list-header><ion-label>Filter by term(s)</ion-label></ion-list-header>
        <ion-item>
          <ion-input v-model="term" placeholder="Search" data-testid="filter-term" />
        </ion-item>

        <ion-list-header><ion-label>Sort by</ion-label></ion-list-header>
        <ion-radio-group v-model="sortKey">
          <ion-item v-for="s in SORT_KEYS" :key="s.key">
            <ion-radio :value="s.key" justify="space-between" :data-testid="`sort-${s.key}`">
              {{ s.label }}
            </ion-radio>
          </ion-item>
        </ion-radio-group>

        <ion-list-header><ion-label>Sort order</ion-label></ion-list-header>
        <ion-radio-group :model-value="ascending" @update:model-value="ascending = $event">
          <ion-item>
            <ion-radio :value="true" justify="space-between" data-testid="sort-asc">Ascending</ion-radio>
          </ion-item>
          <ion-item>
            <ion-radio :value="false" justify="space-between" data-testid="sort-desc">Descending</ion-radio>
          </ion-item>
        </ion-radio-group>

        <ion-list-header><ion-label>Filters by</ion-label></ion-list-header>
        <ion-item v-for="status in ALL_STATUSES" :key="status">
          <ion-checkbox
            :checked="statuses.has(status)"
            justify="space-between"
            :data-testid="`status-${status}`"
            @ionChange="toggleStatus(status, $event.detail.checked)"
          >
            {{ STATUS_LABELS[status] ?? status }}
          </ion-checkbox>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-modal>
</template>
