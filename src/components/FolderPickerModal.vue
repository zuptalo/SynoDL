<script setup lang="ts">
import {
  alertController,
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonIcon,
  IonItem,
  IonLabel,
  IonList,
  IonModal,
  IonNote,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { createOutline, star, starOutline } from 'ionicons/icons';
import { computed, ref, watch } from 'vue';
import { api } from '@/services/api';
import { useDestinationPrefs } from '@/composables/useDestinationPrefs';
import type { Folder } from '@/types/task';

const props = defineProps<{ isOpen: boolean }>();
const emit = defineEmits<{
  // Emits the Download Station destination: the picked path without its
  // leading slash (e.g. "tv-show/Friends").
  (e: 'pick', destination: string): void;
  (e: 'dismiss'): void;
}>();

const { defaultDest, isFavorite, setDefault, toggleFavorite } = useDestinationPrefs();

// Navigation IS selection: tapping a folder enters it, and "Select" confirms
// the folder you are currently in.
const path = ref(''); // '' = share root (nothing selectable yet)
const folders = ref<Folder[]>([]);
const loading = ref(false);
const error = ref('');

// The destination form (no leading slash) — how favorites/default are stored.
const dest = computed(() => path.value.replace(/^\//, ''));

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
  emit('pick', dest.value);
}

// Create a subfolder here, then drill into it so Select stores downloads there.
async function newFolder(): Promise<void> {
  const alert = await alertController.create({
    header: 'New folder',
    inputs: [{ name: 'name', type: 'text', placeholder: 'Folder name' }],
    buttons: [
      { text: 'Cancel', role: 'cancel' },
      { text: 'Create', role: 'confirm' },
    ],
  });
  await alert.present();
  const { role, data } = await alert.onDidDismiss();
  const name = String(data?.values?.name ?? '').trim();
  if (role !== 'confirm' || !name) return;
  try {
    const { folder } = await api.createFolder(path.value, name);
    path.value = folder.path; // drill into the new folder
    await load();
  } catch {
    error.value = 'Could not create the folder.';
  }
}
</script>

<template>
  <ion-modal :is-open="isOpen" @didDismiss="emit('dismiss')">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-button data-testid="folder-cancel" @click="emit('dismiss')">Cancel</ion-button>
          <ion-button v-if="path" data-testid="folder-up" @click="up">Back</ion-button>
        </ion-buttons>
        <ion-title data-testid="folder-title">{{ path || 'Destination' }}</ion-title>
        <ion-buttons slot="end">
          <ion-button v-if="path" data-testid="folder-new" @click="newFolder">
            <ion-icon slot="icon-only" :icon="createOutline" />
          </ion-button>
          <ion-button :disabled="!path" data-testid="folder-confirm" @click="confirm">Select</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content :fullscreen="true">
      <!-- Favorite / default actions for the folder you're currently in. -->
      <div v-if="path" class="destbar">
        <ion-button size="small" fill="clear" data-testid="folder-favorite" @click="toggleFavorite(dest)">
          <ion-icon slot="start" :icon="isFavorite(dest) ? star : starOutline" />
          {{ isFavorite(dest) ? 'Favorited' : 'Favorite' }}
        </ion-button>
        <ion-button
          size="small"
          fill="clear"
          :color="defaultDest === dest ? 'primary' : 'medium'"
          data-testid="folder-default"
          @click="setDefault(dest)"
        >
          {{ defaultDest === dest ? 'Default folder' : 'Set as default' }}
        </ion-button>
      </div>

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
.destbar {
  display: flex;
  gap: 0.5rem;
  padding: 0.25rem 0.5rem;
  border-bottom: 1px solid var(--app-card);
}
</style>
