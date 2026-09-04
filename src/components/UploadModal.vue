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
  IonSegment,
  IonSegmentButton,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { cloudUploadOutline, folderOutline } from 'ionicons/icons';
import { api } from '@/services/api';
import { useUploads } from '@/composables/useUploads';
import { isPlexReady, plexName } from '@/services/title-year';

const props = defineProps<{ isOpen: boolean }>();
const emit = defineEmits<{ (e: 'dismiss'): void; (e: 'uploaded'): void }>();

type Kind = 'movie' | 'tv';

// The queue lives in the composable, not here, so dismissing this sheet leaves a
// running transfer visible in the Tasks list instead of hiding it.
const { enqueue } = useUploads();

const kind = ref<Kind>('movie');
const title = ref('');
const season = ref('');
// Files chosen but not yet sent. Pressing Upload hands them to the queue and
// closes this sheet, so nothing about a running job is tracked here.
const picked = ref<File[]>([]);
const loadError = ref('');
// Titles already on the NAS under the chosen parent. Picking one is what stops
// a near-duplicate folder being created for a show that is already there.
const existing = ref<string[]>([]);
const parents = ref<{ movie: string; tv: string }>({ movie: '', tv: '' });
// Replaced by the server's real limit as soon as the modal opens; this is only
// what to show for the instant before that lands.
const maxMB = ref(10240);

const parentPath = computed(() => (kind.value === 'tv' ? parents.value.tv : parents.value.movie));
// "10240 MB" reads as noise; a cap this size belongs in GB. Whole numbers stay
// whole ("10 GB", not "10.0 GB") and anything under a gigabyte stays in MB.
const maxLabel = computed(() => {
  const mb = maxMB.value;
  if (mb < 1024) return `${mb} MB`;
  const gb = mb / 1024;
  return `${Number.isInteger(gb) ? gb : gb.toFixed(1)} GB`;
});
const anyFiles = computed(() => picked.value.length > 0);
// Refused before a byte leaves the device, and named so it is obvious which one.
const oversized = computed(() => picked.value.filter(tooBig).map((f) => f.name));
const canSend = computed(
  () =>
    anyFiles.value &&
    isPlexReady(title.value) &&
    parentPath.value !== '',
);
// Only nag once there is something to judge — an empty field is not yet "wrong".
const titleNeedsYear = computed(() => title.value.trim() !== '' && !isPlexReady(title.value));

/**
 * Folders already on the NAS, narrowed by whatever has been typed.
 *
 * The library runs to hundreds of titles, so a plain dropdown is unusable on a
 * phone. Filtering as you type turns the Title field into a combobox: type to
 * find an existing show (so a new episode joins it rather than starting a
 * near-duplicate), or keep typing to name something new.
 */
const filteredExisting = computed(() => {
  const q = title.value.trim().toLowerCase();
  if (!existing.value.length) return [];
  const matches = q === '' ? existing.value : existing.value.filter((n) => n.toLowerCase().includes(q));
  // Once the field IS one of the folders, the choice is made — stop suggesting.
  if (matches.length === 1 && matches[0].toLowerCase() === q) return [];
  return matches;
});
// Shown so the user can see where this is going before committing to it.
const preview = computed(() => {
  const t = title.value.trim();
  if (!t || !parentPath.value) return '';
  const n = Number(season.value);
  const seasonPart =
    kind.value === 'tv' && season.value !== '' && Number.isFinite(n)
      ? `/Season ${String(n).padStart(2, '0')}`
      : '';
  // The server names the folder with the Plex convention, so preview that
  // rather than the raw title — otherwise this line promises a folder that is
  // not the one the file lands in.
  return `${parentPath.value}/${plexName(t)}${seasonPart}`;
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
  picked.value = [];
  void loadContext();
});
watch(kind, () => {
  season.value = '';
  void loadExisting();
});

function onPick(e: Event): void {
  const chosen = (e.target as HTMLInputElement).files;
  if (!chosen) return;
  picked.value = Array.from(chosen);
}

const tooBig = (f: File) => f.size > maxMB.value * 1024 * 1024;

function send(): void {
  // Hand the files to the shared queue and close, exactly as adding by URL does.
  // Progress, retry and replace all live in the Tasks list, so keeping this sheet
  // open would only duplicate them and leave the user choosing which to watch.
  enqueue(picked.value.filter((f) => !tooBig(f)), {
    kind: kind.value,
    title: title.value.trim(),
    season: season.value,
  });
  emit('uploaded');
}
</script>

<template>
  <ion-modal :is-open="props.isOpen" @did-dismiss="emit('dismiss')">
    <ion-header>
      <ion-toolbar>
        <ion-title>Upload to library</ion-title>
        <ion-buttons slot="end">
          <ion-button data-testid="upload-cancel" @click="emit('dismiss')">
            Close
          </ion-button>
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
        <ion-item>
          <ion-input
            v-model="title"
            label="Title"
            label-placement="stacked"
            data-testid="upload-title"
            :placeholder="kind === 'tv' ? 'Friends 1994' : 'Dune 2021'"
          />
        </ion-item>
        <!-- Typing filters what is already on the NAS, so an episode can be added
             to an existing show. The whole matching set stays scrollable — an
             earlier version capped it, which quietly made most of the library
             unreachable unless you guessed enough of the name. -->
        <div v-if="filteredExisting.length" class="suggestions">
          <ion-item
            v-for="name in filteredExisting"
            :key="name"
            button
            :detail="false"
            class="suggestion"
            data-testid="upload-existing"
            @click="title = name"
          >
            <ion-icon slot="start" :icon="folderOutline" size="small" />
            <ion-label>{{ name }}</ion-label>
          </ion-item>
        </div>
        <ion-item v-if="titleNeedsYear" lines="none">
          <ion-note color="warning" data-testid="upload-title-hint">
            Add the release year so your media server can identify it — e.g.
            <strong>{{ kind === 'tv' ? 'Friends 1994' : 'Dune 2021' }}</strong>.
          </ion-note>
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
      <ion-note v-if="oversized.length" class="cap" color="danger">
        Too large for the {{ maxLabel }} limit: {{ oversized.join(', ') }}.
      </ion-note>
      <ion-note class="cap">
        Video, subtitle, artwork and .nfo files, up to {{ maxLabel }} each. The title is used to
        name the folder, so it is required.
      </ion-note>

      <ion-list v-if="anyFiles">
        <ion-list-header><ion-label>Files</ion-label></ion-list-header>
        <ion-item v-for="f in picked" :key="f.name">
          <ion-label><h3>{{ f.name }}</h3></ion-label>
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
    </ion-content>
  </ion-modal>
</template>

<style scoped>
.suggestion {
  --min-height: 40px;
}
/* Bounded so a long match list scrolls within itself instead of pushing the
   Upload button off the sheet. */
.suggestions {
  max-height: 34vh;
  overflow-y: auto;
}
.hint {
  display: block;
  padding: 4px 16px 0;
  font-size: 0.78rem;
}
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
