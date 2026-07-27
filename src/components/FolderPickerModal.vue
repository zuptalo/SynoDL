<script setup lang="ts">
import {
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonItem,
  IonLabel,
  IonList,
  IonModal,
  IonNote,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { ref, watch } from 'vue';
import { api } from '@/services/api';
import type { Folder } from '@/types/task';

const props = defineProps<{ isOpen: boolean }>();
const emit = defineEmits<{
  // Emits the Download Station destination: the picked path without its
  // leading slash (e.g. "tv-show/Friends").
  (e: 'pick', destination: string): void;
  (e: 'dismiss'): void;
}>();

// Navigation IS selection: tapping a folder enters it, and "Select" confirms
// the folder you are currently in. One gesture, no separate radio state —
// selecting a leaf without entering it isn't a thing users need.
const path = ref(''); // '' = share root (nothing selectable yet)
const folders = ref<Folder[]>([]);
const loading = ref(false);
const error = ref('');

async function load(): Promise<void> {
  loading.value = true;
  error.value = '';
  try {
    const res = path.value === '' ? await api.shares() : await api.listFolder(path.value);
    folders.value = res.folders;
  } catch {
    error.value = 'Could not load folders.';
    folders.value = [];
  } finally {
    loading.value = false;
  }
}

watch(
  () => props.isOpen,
  (open) => {
    if (open) {
      path.value = '';
      void load();
    }
  },
);

function drillInto(folder: Folder): void {
  path.value = folder.path;
  void load();
}

function up(): void {
  path.value = path.value.slice(0, path.value.lastIndexOf('/'));
  void load();
}

function confirm(): void {
  if (!path.value) return;
  emit('pick', path.value.replace(/^\//, ''));
}
</script>

<template>
  <ion-modal :is-open="isOpen" @didDismiss="emit('dismiss')">
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-button v-if="path" data-testid="folder-up" @click="up">Back</ion-button>
        </ion-buttons>
        <ion-title>{{ path || 'Destination' }}</ion-title>
        <ion-buttons slot="end">
          <ion-button :disabled="!path" data-testid="folder-confirm" @click="confirm">
            Select
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content>
      <div v-if="loading" class="center"><ion-spinner name="crescent" /></div>
      <ion-note v-else-if="error" color="danger" class="ion-padding">{{ error }}</ion-note>
      <ion-list v-else>
        <ion-item
          v-for="folder in folders"
          :key="folder.path"
          button
          :detail="true"
          data-testid="folder-item"
          @click="drillInto(folder)"
        >
          <ion-label>{{ folder.name }}</ion-label>
        </ion-item>
        <ion-item v-if="folders.length === 0" lines="none">
          <ion-label color="medium">No subfolders — Select uses this folder.</ion-label>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-modal>
</template>

<style scoped>
.center {
  display: flex;
  justify-content: center;
  padding-top: 20vh;
}
</style>
