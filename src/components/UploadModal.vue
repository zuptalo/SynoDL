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
import { api, type UploadKind } from '@/services/api';
import { useUploads } from '@/composables/useUploads';
import { isPlexReady, plexName } from '@/services/title-year';

const props = defineProps<{ isOpen: boolean }>();
const emit = defineEmits<{ (e: 'dismiss'): void; (e: 'uploaded'): void }>();

type Kind = UploadKind;

// Music is filed by artist and album rather than by one title, so the sheet asks
// different questions for it (spec 1040). Kept as one predicate rather than
// repeated comparisons, so adding a third music kind cannot half-work.
const MUSIC_KINDS: Kind[] = ['music', 'music-video'];

// The queue lives in the composable, not here, so dismissing this sheet leaves a
// running transfer visible in the Tasks list instead of hiding it.
const { enqueue } = useUploads();

const kind = ref<Kind>('movie');
const title = ref('');
const season = ref('');
// Music (spec 1040). One set of details for the whole upload: the audio, its
// lyrics and its artwork all describe the same track.
const track = ref('');
const artist = ref('');
const album = ref('');
// Files chosen but not yet sent. Pressing Upload hands them to the queue and
// closes this sheet, so nothing about a running job is tracked here.
const picked = ref<File[]>([]);
const loadError = ref('');
// Titles already on the NAS under the chosen parent. Picking one is what stops
// a near-duplicate folder being created for a show that is already there.
const existing = ref<string[]>([]);
const parents = ref<Record<Kind, string>>({ movie: '', tv: '', music: '', 'music-video': '' });
// Replaced by the server's real limit as soon as the modal opens; this is only
// what to show for the instant before that lands.
const maxMB = ref(10240);

const isMusic = computed(() => MUSIC_KINDS.includes(kind.value));
const parentPath = computed(() => parents.value[kind.value] ?? '');
// A kind whose library is not configured is not offered at all, rather than
// offered and then failing at the last step (FR-002).
const kinds = computed(() =>
  (['movie', 'tv', 'music', 'music-video'] as Kind[]).filter((k) => parents.value[k] !== ''),
);
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
const canSend = computed(() => {
  if (!anyFiles.value || parentPath.value === '') return false;
  // A track needs a name and somebody to belong to. The album is optional and
  // falls back to Singles, exactly as a download with no album does.
  if (isMusic.value) return track.value.trim() !== '' && artist.value.trim() !== '';
  return isPlexReady(title.value);
});
// Only nag once there is something to judge — an empty field is not yet "wrong".
const titleNeedsYear = computed(
  () => !isMusic.value && title.value.trim() !== '' && !isPlexReady(title.value),
);

/**
 * Folders already on the NAS, narrowed by whatever has been typed.
 *
 * The library runs to hundreds of titles, so a plain dropdown is unusable on a
 * phone. Filtering as you type turns the Title field into a combobox: type to
 * find an existing show (so a new episode joins it rather than starting a
 * near-duplicate), or keep typing to name something new.
 */
const filteredExisting = computed(() => {
  const q = (isMusic.value ? artist.value : title.value).trim().toLowerCase();
  if (!existing.value.length) return [];
  const matches = q === '' ? existing.value : existing.value.filter((n) => n.toLowerCase().includes(q));
  // Once the field IS one of the folders, the choice is made — stop suggesting.
  if (matches.length === 1 && matches[0].toLowerCase() === q) return [];
  return matches;
});
// Shown so the user can see where this is going before committing to it.
const preview = computed(() => {
  if (!parentPath.value) return '';
  if (isMusic.value) {
    const a = artist.value.trim();
    const t = track.value.trim();
    if (!a || !t) return '';
    // The same layout a downloaded track gets, Singles and all, so what this
    // line promises is what the server composes.
    return `${parentPath.value}/${a}/${album.value.trim() || 'Singles'}/${t}`;
  }
  const t = title.value.trim();
  if (!t) return '';
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
    // Film and TV parents are inherited from a download source; music has no
    // source and comes from the operator's own setting (spec 1040).
    const music = await api.getMusicLibraries().catch(() => ({ music: '', musicVideo: '' }));
    parents.value = {
      movie: st.moviesParent ?? '',
      tv: st.tvParent ?? '',
      music: music.music ?? '',
      'music-video': music.musicVideo ?? '',
    };
    if (!kinds.value.length) {
      loadError.value = 'No library folder is configured yet.';
    } else if (!kinds.value.includes(kind.value)) {
      kind.value = kinds.value[0];
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
  track.value = '';
  artist.value = '';
  album.value = '';
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
    track: track.value.trim(),
    artist: artist.value.trim(),
    album: album.value.trim(),
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

      <!-- Only the kinds whose library is actually configured (FR-002). An
           option that cannot work is worse than an absent one: it looks like a
           capability right up until the last step. -->
      <ion-segment v-model="kind" data-testid="upload-kind">
        <ion-segment-button v-if="kinds.includes('movie')" value="movie" data-testid="upload-kind-movie">
          <ion-label>Movie</ion-label>
        </ion-segment-button>
        <ion-segment-button v-if="kinds.includes('tv')" value="tv" data-testid="upload-kind-tv">
          <ion-label>TV show</ion-label>
        </ion-segment-button>
        <ion-segment-button v-if="kinds.includes('music')" value="music" data-testid="upload-kind-music">
          <ion-label>Music</ion-label>
        </ion-segment-button>
        <ion-segment-button v-if="kinds.includes('music-video')" value="music-video" data-testid="upload-kind-music-video">
          <ion-label>Music video</ion-label>
        </ion-segment-button>
      </ion-segment>

      <!-- Music is filed by artist and album rather than by one title, so it
           asks different questions (spec 1040). The three answers describe the
           track, and every file in the upload uses them: the audio, its lyrics
           and its artwork are all the same track. -->
      <ion-list v-if="isMusic">
        <ion-item>
          <ion-input
            v-model="track"
            label="Track name"
            label-placement="stacked"
            autocapitalize="words"
            data-testid="upload-track"
            placeholder="Lucente"
          />
        </ion-item>
        <ion-item>
          <ion-input
            v-model="artist"
            label="Artist"
            label-placement="stacked"
            autocapitalize="words"
            data-testid="upload-artist"
            placeholder="Anyma"
          />
        </ion-item>
        <!-- Artists already in the library, filtered as you type, so a new track
             joins the folder that is there rather than starting a near-duplicate
             beside it. The same combobox the title field uses. -->
        <div v-if="filteredExisting.length" class="suggestions">
          <ion-item
            v-for="name in filteredExisting"
            :key="name"
            button
            :detail="false"
            class="suggestion"
            data-testid="upload-existing"
            @click="artist = name"
          >
            <ion-icon slot="start" :icon="folderOutline" size="small" />
            <ion-label>{{ name }}</ion-label>
          </ion-item>
        </div>
        <ion-item>
          <ion-input
            v-model="album"
            label="Album (optional)"
            label-placement="stacked"
            autocapitalize="words"
            data-testid="upload-album"
            placeholder="The End Of Genesys"
          />
        </ion-item>
        <ion-item lines="none">
          <ion-note data-testid="upload-music-hint">
            Pick the audio and, if you have them, its lyrics and a thumbnail —
            they are all named after the track so your media server pairs them.
            With no album it is filed under <strong>Singles</strong>.
          </ion-note>
        </ion-item>
      </ion-list>

      <ion-list v-else>
        <ion-item>
          <!-- Capitalise each word as it is typed, the way a title is normally
               written. This is the KEYBOARD's default rather than a transform on
               the value: rewriting what was typed would fight anyone entering an
               acronym or a title that is genuinely lower-case, and the folder
               name is the user's to decide. -->
          <ion-input
            v-model="title"
            label="Title"
            label-placement="stacked"
            autocapitalize="words"
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
        <template v-if="isMusic">
          {{ kind === 'music' ? 'Audio' : 'Video' }}, lyrics and artwork files, up to
          {{ maxLabel }} each. The track name and artist are used to name the file and its
          folders, so both are required.
        </template>
        <template v-else>
          Video, subtitle, artwork and .nfo files, up to {{ maxLabel }} each. The title is used
          to name the folder, so it is required.
        </template>
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
