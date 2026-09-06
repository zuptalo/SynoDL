<script setup lang="ts">
import {
  alertController,
  IonButton,
  IonButtons,
  IonChip,
  IonContent,
  IonHeader,
  IonIcon,
  IonInput,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonModal,
  IonNote,
  IonSegment,
  IonSegmentButton,
  IonSelect,
  IonSelectOption,
  IonTextarea,
  IonTitle,
  IonToggle,
  IonToolbar,
} from '@ionic/vue';
import { createOutline, folderOutline } from 'ionicons/icons';
import { computed, ref, watch } from 'vue';
import { api, ApiError, type StatCategory } from '@/services/api';
import { batch, extractUrls } from '@/services/url-detect';
import { messageForError } from '@/services/syno-errors';
import { useDestinationPrefs } from '@/composables/useDestinationPrefs';
import { useSession } from '@/composables/useSession';
import { useYtdl } from '@/composables/useYtdl';
import FolderPickerModal from '@/components/FolderPickerModal.vue';

const { defaultDest, favorites } = useDestinationPrefs();
const { mode } = useSession();
// The media category only feeds statistics, which exist in stateful mode.
const stateful = computed(() => mode.value === 'stateful');

const props = defineProps<{ isOpen: boolean }>();
const emit = defineEmits<{
  (e: 'created'): void;
  (e: 'dismiss'): void;
}>();

const text = ref('');
const destination = ref('');
const pickerOpen = ref(false);
const withCredentials = ref(false);
const username = ref('');
const password = ref('');
const withExtract = ref(false);
const unzipPassword = ref('');
const file = ref<File | null>(null);
// 'auto' lets the server classify from the folder + file type; a specific choice
// overrides that. Only sent (and shown) in stateful mode.
const category = ref<StatCategory | 'auto'>('auto');
const busy = ref(false);
const error = ref('');
const progress = ref('');

// Download Station caps how many URLs one create request accepts, so a bulk
// paste is sent in batches (spec 1005).
const BATCH_SIZE = 10;

const urls = computed(() => extractUrls(text.value));

// YouTube links can go somewhere Download Station cannot follow (spec 0012):
// straight into the music or music-video library, fetched by a worker. The
// choice only appears when there IS a YouTube link in the box and the server
// can actually run workers, so the ordinary "send this to the NAS" flow is
// untouched for everyone else.
const YOUTUBE_HOSTS = [
  'youtube.com',
  'www.youtube.com',
  'm.youtube.com',
  'music.youtube.com',
  'youtu.be',
];
const youtubeUrls = computed(() =>
  urls.value.filter((u) => {
    try {
      return YOUTUBE_HOSTS.includes(new URL(u).hostname.toLowerCase());
    } catch {
      return false;
    }
  }),
);
const { available: ytdlAvailable, submit: submitYtdl } = useYtdl();
const ytdlMode = ref<'nas' | 'music' | 'music-video'>('nas');
const sendingToLibrary = computed(
  () => ytdlMode.value !== 'nas' && youtubeUrls.value.length > 0 && ytdlAvailable.value,
);
const canSubmit = computed(() => !busy.value && (urls.value.length > 0 || file.value !== null));

watch(
  () => props.isOpen,
  (open) => {
    if (open) {
      text.value = '';
      destination.value = defaultDest.value; // pre-fill with the saved default
      withCredentials.value = false;
      username.value = '';
      password.value = '';
      withExtract.value = false;
      unzipPassword.value = '';
      file.value = null;
      category.value = 'auto';
      error.value = '';
    }
  },
);

// Append whatever's on the clipboard to the URL box, ALWAYS starting a fresh
// line so a second bulk can never glue onto the first (which made the whole
// second batch count as one link). Surfaces a hint if the clipboard can't be
// read (iOS is finicky) instead of failing silently.
async function pasteFromClipboard(): Promise<void> {
  error.value = '';
  let clip = '';
  try {
    clip = (await navigator.clipboard.readText()).trim();
  } catch {
    error.value = 'Could not read the clipboard — long-press the box and paste manually.';
    return;
  }
  if (!clip) return;
  const base = text.value.replace(/\s+$/, '');
  text.value = base ? `${base}\n${clip}` : clip;
}

function clearUrls(): void {
  text.value = '';
  error.value = '';
}

function onFileChosen(event: Event): void {
  const input = event.target as HTMLInputElement;
  file.value = input.files?.[0] ?? null;
}

// Create a subfolder inside the current destination and make it the destination,
// without leaving the new-task screen (spec 1009).
async function newFolderHere(): Promise<void> {
  if (!destination.value) return;
  const alert = await alertController.create({
    header: 'New folder',
    subHeader: `Inside ${destination.value}`,
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
    const { folder } = await api.createFolder(`/${destination.value}`, name);
    destination.value = folder.path.replace(/^\//, ''); // select the new folder
  } catch (e) {
    error.value = messageForError(e);
  }
}

async function submit(): Promise<void> {
  busy.value = true;
  error.value = '';
  progress.value = '';
  try {
    // A media library is a different destination entirely: it does not touch
    // the NAS task path, so this returns early rather than falling through into
    // the Download Station options below.
    if (sendingToLibrary.value) {
      const mode = ytdlMode.value as 'music' | 'music-video';
      let added = 0;
      for (const url of youtubeUrls.value) {
        await submitYtdl(url, mode);
        added += 1;
        if (youtubeUrls.value.length > 1) {
          progress.value = `Started ${added} of ${youtubeUrls.value.length}…`;
        }
      }
      emit('created');
      return;
    }
    if (urls.value.length > 0) {
      const opts = {
        destination: destination.value || undefined,
        username: withCredentials.value ? username.value : undefined,
        password: withCredentials.value ? password.value : undefined,
        unzipPassword: withExtract.value ? unzipPassword.value : undefined,
        category: stateful.value ? category.value : undefined,
      };
      const chunks = batch(urls.value, BATCH_SIZE);
      let added = 0;
      for (const chunk of chunks) {
        // Sequential so a mid-batch failure reports how many already landed
        // rather than firing them all and losing the boundary.
        await api.createTaskURIs(chunk, opts);
        added += chunk.length;
        if (chunks.length > 1) progress.value = `Added ${added} of ${urls.value.length}…`;
      }
    }
    if (file.value) {
      await api.createTaskFile(file.value, {
        destination: destination.value || undefined,
        unzipPassword: withExtract.value ? unzipPassword.value : undefined,
        category: stateful.value ? category.value : undefined,
      });
    }
    emit('created');
  } catch (e) {
    error.value =
      e instanceof ApiError && e.status === 413
        ? 'That torrent file is too large to upload.'
        : messageForError(e);
  } finally {
    busy.value = false;
    progress.value = '';
  }
}
</script>

<template>
  <ion-modal :is-open="isOpen" @didDismiss="emit('dismiss')">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-button data-testid="newtask-cancel" @click="emit('dismiss')">Cancel</ion-button>
        </ion-buttons>
        <ion-title>New task</ion-title>
        <ion-buttons slot="end">
          <ion-button :disabled="!canSubmit" data-testid="newtask-submit" @click="submit">
            Add task
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content>
      <ion-list>
        <ion-list-header><ion-label>Destination</ion-label></ion-list-header>
        <!-- Up to 4 favorite folders for one-tap selection (spec 1006). -->
        <div v-if="favorites.length" class="favorites" data-testid="newtask-favorites">
          <ion-chip
            v-for="fav in favorites"
            :key="fav"
            :color="destination === fav ? 'primary' : undefined"
            :outline="destination !== fav"
            data-testid="newtask-favorite"
            @click="destination = fav"
          >
            <ion-icon :icon="folderOutline" />
            <ion-label>{{ fav }}</ion-label>
          </ion-chip>
        </div>
        <ion-item button :detail="true" data-testid="newtask-destination" @click="pickerOpen = true">
          <ion-label>{{ destination || 'Default folder' }}</ion-label>
        </ion-item>
        <!-- Quick "new subfolder here" without opening the picker (spec 1009). -->
        <ion-item v-if="destination" button :detail="false" data-testid="newtask-newfolder" @click="newFolderHere">
          <ion-icon slot="start" :icon="createOutline" color="primary" />
          <ion-label color="primary">New folder in “{{ destination }}”</ion-label>
        </ion-item>

        <!-- Media category for statistics (stateful only). Auto = classify from
             the folder + file type; a choice here overrides that. -->
        <ion-item v-if="stateful">
          <ion-select v-model="category" label="Category" interface="popover" data-testid="newtask-category">
            <ion-select-option value="auto">Auto-detect</ion-select-option>
            <ion-select-option value="movie">Movie</ion-select-option>
            <ion-select-option value="series">TV series</ion-select-option>
            <ion-select-option value="anime">Anime</ion-select-option>
            <ion-select-option value="musicVideo">Music video</ion-select-option>
            <ion-select-option value="music">Music</ion-select-option>
            <ion-select-option value="other">Other</ion-select-option>
          </ion-select>
        </ion-item>

        <ion-list-header><ion-label>Task URL(s)</ion-label></ion-list-header>
        <ion-item>
          <!-- Explicit :value + @ionInput (not v-model) so the detected-link
               count always reflects exactly what's in the box, including pastes. -->
          <ion-textarea
            :value="text"
            :rows="4"
            :auto-grow="true"
            placeholder="Enter one or many task URLs"
            data-testid="newtask-urls"
            @ionInput="text = String($event.target.value ?? '')"
          />
        </ion-item>
        <ion-item lines="none">
          <ion-button fill="clear" size="small" data-testid="newtask-paste" @click="pasteFromClipboard">
            Paste from clipboard
          </ion-button>
          <ion-button
            v-if="text"
            fill="clear"
            size="small"
            color="medium"
            data-testid="newtask-clear"
            @click="clearUrls"
          >
            Clear
          </ion-button>
          <ion-note slot="end" data-testid="newtask-count">
            {{ urls.length }} link{{ urls.length === 1 ? '' : 's' }} detected
          </ion-note>
        </ion-item>

        <!-- Only for YouTube links, and only where the server can run workers. -->
        <template v-if="youtubeUrls.length > 0 && ytdlAvailable">
          <ion-list-header><ion-label>Send to</ion-label></ion-list-header>
          <ion-item lines="none">
            <ion-segment
              :value="ytdlMode"
              data-testid="ytdl-mode"
              @ionChange="ytdlMode = ($event.detail.value ?? 'nas') as 'nas' | 'music' | 'music-video'"
            >
              <ion-segment-button value="nas" data-testid="ytdl-mode-nas">
                <ion-label>Download Station</ion-label>
              </ion-segment-button>
              <ion-segment-button value="music" data-testid="ytdl-mode-music">
                <ion-label>Music</ion-label>
              </ion-segment-button>
              <ion-segment-button value="music-video" data-testid="ytdl-mode-video">
                <ion-label>Music video</ion-label>
              </ion-segment-button>
            </ion-segment>
          </ion-item>
          <ion-item v-if="sendingToLibrary" lines="none">
            <ion-note data-testid="ytdl-hint">
              {{ youtubeUrls.length }} YouTube link{{ youtubeUrls.length === 1 ? '' : 's' }} will be
              saved to your {{ ytdlMode === 'music' ? 'music' : 'music video' }} library. A playlist
              or channel link fetches everything in it that is a full-length track.
            </ion-note>
          </ion-item>
        </template>

        <ion-list-header><ion-label>Task file</ion-label></ion-list-header>
        <ion-item>
          <input
            type="file"
            accept=".torrent,.nzb,application/x-bittorrent"
            data-testid="newtask-file"
            @change="onFileChosen"
          />
        </ion-item>

        <ion-list-header><ion-label>Extra</ion-label></ion-list-header>
        <ion-item>
          <ion-toggle v-model="withCredentials" data-testid="newtask-credentials-toggle">
            Add credentials
          </ion-toggle>
        </ion-item>
        <template v-if="withCredentials">
          <ion-item>
            <ion-input v-model="username" label="Username" label-placement="stacked" autocapitalize="off" />
          </ion-item>
          <ion-item>
            <ion-input v-model="password" label="Password" label-placement="stacked" type="password" />
          </ion-item>
        </template>
        <ion-item>
          <ion-toggle v-model="withExtract" data-testid="newtask-extract-toggle">
            Extract password
          </ion-toggle>
        </ion-item>
        <ion-item v-if="withExtract">
          <ion-input v-model="unzipPassword" label="Archive password" label-placement="stacked" />
        </ion-item>
      </ion-list>

      <ion-note v-if="progress" color="medium" class="ion-padding" data-testid="newtask-progress">
        {{ progress }}
      </ion-note>
      <ion-note v-if="error" color="danger" class="ion-padding" data-testid="newtask-error">
        {{ error }}
      </ion-note>
    </ion-content>

    <FolderPickerModal
      :is-open="pickerOpen"
      :initial-dest="destination"
      @pick="(d) => { destination = d; pickerOpen = false; }"
      @dismiss="pickerOpen = false"
    />
  </ion-modal>
</template>
