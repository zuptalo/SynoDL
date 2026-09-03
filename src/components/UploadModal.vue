<script setup lang="ts">
/**
 * Upload a file straight into the library (spec 1022).
 *
 * The destination is never typed as a path. The user says what KIND of thing
 * this is, names the title (or picks a show already on the NAS), and optionally
 * a season; the server composes the folder from those. That is what keeps an
 * uploaded title indistinguishable from a downloaded one afterwards.
 */
import { computed, ref, watch } from 'vue';
import {
  IonButton,
  IonButtons,
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
  IonProgressBar,
  IonSegment,
  IonSegmentButton,
  IonSelect,
  IonSelectOption,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { checkmarkCircle, cloudUploadOutline, warningOutline } from 'ionicons/icons';
import { ApiError, api, type UploadResult } from '@/services/api';

const props = defineProps<{ isOpen: boolean }>();
const emit = defineEmits<{ (e: 'dismiss'): void; (e: 'uploaded'): void }>();

type Kind = 'movie' | 'tv';
type Row = {
  file: File;
  progress: number;
  state: 'waiting' | 'sending' | 'done' | 'failed';
  message: string;
};

const kind = ref<Kind>('movie');
const title = ref('');
const season = ref('');
const files = ref<Row[]>([]);
const busy = ref(false);
const loadError = ref('');
// Titles already on the NAS under the chosen parent. Picking one is what stops
// a near-duplicate folder being created for a show that is already there.
const existing = ref<string[]>([]);
const parents = ref<{ movie: string; tv: string }>({ movie: '', tv: '' });
// Replaced by the server's real limit as soon as the modal opens; this is only
// what to show for the instant before that lands.
const maxMB = ref(2048);
let cancelCurrent: (() => void) | null = null;

const parentPath = computed(() => (kind.value === 'tv' ? parents.value.tv : parents.value.movie));
const anyFiles = computed(() => files.value.length > 0);
const canSend = computed(
  () => !busy.value && anyFiles.value && title.value.trim() !== '' && parentPath.value !== '',
);
// Shown so the user can see where this is going before committing to it.
const preview = computed(() => {
  const t = title.value.trim();
  if (!t || !parentPath.value) return '';
  const n = Number(season.value);
  const seasonPart =
    kind.value === 'tv' && season.value !== '' && Number.isFinite(n)
      ? `/Season ${String(n).padStart(2, '0')}`
      : '';
  return `${parentPath.value}/${t}${seasonPart}`;
});

async function loadContext(): Promise<void> {
  loadError.value = '';
  try {
    const cfg = await api.config();
    if (cfg.uploadMaxMB && cfg.uploadMaxMB > 0) maxMB.value = cfg.uploadMaxMB;
    const st = await api.getSourceStatus();
    parents.value = { movie: st.moviesParent ?? '', tv: st.tvParent ?? '' };
    if (!parents.value.movie && !parents.value.tv) {
      loadError.value = 'No movie or TV folder is configured yet.';
    }
  } catch {
    loadError.value = 'Could not read the library folders.';
  }
  await loadExisting();
}

async function loadExisting(): Promise<void> {
  existing.value = [];
  if (!parentPath.value) return;
  try {
    const { folders } = await api.listFolder(`/${parentPath.value}`);
    existing.value = folders.map((f) => f.name).sort((a, b) => a.localeCompare(b));
  } catch {
    // Not fatal: the user can still type a title.
    existing.value = [];
  }
}

watch(() => props.isOpen, (open) => {
  if (!open) return;
  kind.value = 'movie';
  title.value = '';
  season.value = '';
  files.value = [];
  busy.value = false;
  void loadContext();
});
watch(kind, () => {
  season.value = '';
  void loadExisting();
});

function onPick(e: Event): void {
  const picked = (e.target as HTMLInputElement).files;
  if (!picked) return;
  files.value = Array.from(picked).map((file) => ({
    file,
    progress: 0,
    state: 'waiting' as const,
    message: '',
  }));
}

const tooBig = (f: File) => f.size > maxMB.value * 1024 * 1024;

function explain(e: unknown): string {
  if (!(e instanceof ApiError)) return 'Upload failed.';
  switch (e.code) {
    case 'file_exists':
      return 'A file of that name is already there — nothing was overwritten.';
    case 'destination_forbidden':
      return 'You do not have access to that folder.';
    case 'parent_unset':
      return 'No movie or TV folder is configured.';
    case 'cancelled':
      return 'Cancelled.';
    case 'offline':
      return 'Lost connection.';
    default:
      return e.message || 'Upload failed.';
  }
}

async function send(): Promise<void> {
  busy.value = true;
  let anyOk = false;
  for (const row of files.value) {
    if (row.state === 'done') continue;
    // Refused before a single byte leaves the device.
    if (tooBig(row.file)) {
      row.state = 'failed';
      row.message = `Larger than the ${maxMB.value} MB limit.`;
      continue;
    }
    row.state = 'sending';
    row.progress = 0;
    const { promise, cancel } = api.uploadFile(
      { kind: kind.value, title: title.value.trim(), season: season.value, file: row.file },
      (fraction) => {
        row.progress = fraction;
      },
    );
    cancelCurrent = cancel;
    try {
      const res: UploadResult = await promise;
      row.state = 'done';
      row.progress = 1;
      row.message = res.destination;
      anyOk = true;
    } catch (e) {
      // One file failing must not abandon the rest.
      row.state = 'failed';
      row.message = explain(e);
    } finally {
      cancelCurrent = null;
    }
  }
  busy.value = false;
  if (anyOk) emit('uploaded');
}

function stop(): void {
  cancelCurrent?.();
}
</script>

<template>
  <ion-modal :is-open="props.isOpen" @did-dismiss="emit('dismiss')">
    <ion-header>
      <ion-toolbar>
        <ion-title>Upload to library</ion-title>
        <ion-buttons slot="end">
          <ion-button data-testid="upload-cancel" @click="emit('dismiss')">Close</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content class="ion-padding">
      <ion-note v-if="loadError" color="danger">{{ loadError }}</ion-note>

      <ion-segment v-model="kind" data-testid="upload-kind">
        <ion-segment-button value="movie"><ion-label>Movie</ion-label></ion-segment-button>
        <ion-segment-button value="tv"><ion-label>TV show</ion-label></ion-segment-button>
      </ion-segment>

      <ion-list>
        <!-- Picking a show already on the NAS uses that exact folder, so a new
             episode joins the show instead of starting a near-duplicate. -->
        <ion-item v-if="existing.length">
          <ion-select
            label="Already in your library"
            label-placement="stacked"
            interface="popover"
            placeholder="or type a new title below"
            data-testid="upload-existing"
            @ion-change="title = String($event.detail.value ?? '')"
          >
            <ion-select-option v-for="name in existing" :key="name" :value="name">
              {{ name }}
            </ion-select-option>
          </ion-select>
        </ion-item>

        <ion-item>
          <ion-input
            v-model="title"
            label="Title"
            label-placement="stacked"
            data-testid="upload-title"
            :placeholder="kind === 'tv' ? 'Friends 1994' : 'Dune 2021'"
          />
        </ion-item>
        <ion-item v-if="kind === 'tv'">
          <ion-input
            v-model="season"
            type="number"
            inputmode="numeric"
            min="0"
            label="Season (optional)"
            label-placement="stacked"
            data-testid="upload-season"
            placeholder="1"
          />
        </ion-item>
      </ion-list>

      <ion-note v-if="preview" class="preview" data-testid="upload-preview">
        Goes to <strong>{{ preview }}</strong>
      </ion-note>

      <ion-item lines="none" class="picker">
        <ion-icon slot="start" :icon="cloudUploadOutline" />
        <input type="file" multiple data-testid="upload-input" @change="onPick" />
      </ion-item>
      <ion-note class="cap">
        Video, subtitle, artwork and .nfo files, up to {{ maxMB }} MB each. The title is used to
        name the folder, so it is required.
      </ion-note>

      <ion-list v-if="anyFiles">
        <ion-list-header><ion-label>Files</ion-label></ion-list-header>
        <ion-item v-for="row in files" :key="row.file.name">
          <ion-label>
            <h3>{{ row.file.name }}</h3>
            <p v-if="row.message" :class="{ bad: row.state === 'failed' }">{{ row.message }}</p>
            <ion-progress-bar
              v-if="row.state === 'sending'"
              :value="row.progress"
              data-testid="upload-progress"
            />
          </ion-label>
          <ion-icon
            v-if="row.state === 'done'"
            slot="end"
            :icon="checkmarkCircle"
            color="success"
          />
          <ion-icon
            v-else-if="row.state === 'failed'"
            slot="end"
            :icon="warningOutline"
            color="danger"
          />
        </ion-item>
      </ion-list>

      <ion-button
        expand="block"
        class="go"
        :disabled="!canSend"
        data-testid="upload-send"
        @click="send"
      >
        Upload
      </ion-button>
      <ion-button v-if="busy" expand="block" fill="clear" color="medium" @click="stop">
        Stop
      </ion-button>
    </ion-content>
  </ion-modal>
</template>

<style scoped>
.preview {
  display: block;
  margin: 10px 2px;
  font-size: 0.85rem;
}
.picker {
  margin-top: 8px;
}
.cap {
  display: block;
  margin: 6px 2px 0;
  font-size: 0.78rem;
}
.go {
  margin-top: 18px;
}
.bad {
  color: var(--ion-color-danger);
}
</style>
