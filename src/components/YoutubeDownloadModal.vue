<script setup lang="ts">
/**
 * Saving a YouTube link to a media library (spec 1033).
 *
 * A separate sheet from NewTaskModal rather than a mode inside it. The defect
 * that produced this spec was one sheet trying to be two things: its Destination
 * picker, category and file field are all meaningless for a library download,
 * yet they sat at the top still looking interactive. Expressing that as a
 * conditional inside the same component would have preserved the problem in the
 * code while merely hiding it in the UI.
 *
 * So this asks for exactly two things, and the ABSENCE of a folder picker is
 * itself the signal: you do not choose where these go, the mode does, because
 * the libraries are configured once by the operator.
 */
import { computed, ref, watch } from 'vue';
import {
  IonButton,
  IonButtons,
  IonIcon,
  IonContent,
  IonHeader,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonModal,
  IonNote,
  IonSegment,
  IonSegmentButton,
  IonTextarea,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { clipboardOutline } from 'ionicons/icons';
import { ApiError } from '@/services/api';
import { extractUrls } from '@/services/url-detect';
import { useYtdl } from '@/composables/useYtdl';

const props = defineProps<{ isOpen: boolean }>();
const emit = defineEmits<{
  (e: 'created'): void;
  (e: 'dismiss'): void;
}>();

const { submit: submitYtdl } = useYtdl();

const text = ref('');
// No preselected mode, deliberately. Defaulting to Music would be convenient
// and would also be exactly the class of bug this spec exists to remove: a
// default quietly deciding the thing the user came here to decide.
const mode = ref<'music' | 'music-video' | ''>('');
const busy = ref(false);
const error = ref('');
const progress = ref('');

const urls = computed(() => extractUrls(text.value));
const canSubmit = computed(() => !busy.value && urls.value.length > 0 && mode.value !== '');

// Reopening should not inherit the last attempt's error or half-typed link.
watch(
  () => props.isOpen,
  (open) => {
    if (!open) return;
    text.value = '';
    mode.value = '';
    error.value = '';
    progress.value = '';
  },
);

/**
 * Append the clipboard to the link box (spec 1037, FR-007).
 *
 * Same shape as the general new-task sheet's paste: a fresh line each time, so
 * a second paste can never glue onto the first and make two links read as one.
 * Reading the clipboard is restricted on iOS, so a refusal says so and points at
 * long-press rather than failing silently.
 */
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

async function submit(): Promise<void> {
  busy.value = true;
  error.value = '';
  progress.value = '';
  const chosen = mode.value as 'music' | 'music-video';
  let started = 0;
  try {
    for (const url of urls.value) {
      await submitYtdl(url, chosen);
      started += 1;
      if (urls.value.length > 1) progress.value = `Started ${started} of ${urls.value.length}…`;
    }
    emit('created');
  } catch (e) {
    // Report how far it got. With several links, "it failed" alone leaves the
    // user unable to tell which ones are already running.
    const detail =
      e instanceof ApiError && e.status === 400
        ? 'That link is not a supported YouTube address.'
        : e instanceof ApiError && e.status === 409
          ? 'That link is already downloading.'
          : e instanceof ApiError && e.status === 503
            ? 'Saving to a media library is not set up on this server.'
            : 'Could not start the download.';
    error.value = started > 0 ? `${detail} ${started} of ${urls.value.length} started.` : detail;
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <ion-modal :is-open="isOpen" @didDismiss="emit('dismiss')">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-button data-testid="ytdl-cancel" @click="emit('dismiss')">Cancel</ion-button>
        </ion-buttons>
        <ion-title>From YouTube</ion-title>
        <ion-buttons slot="end">
          <ion-button :disabled="!canSubmit" data-testid="ytdl-submit" @click="submit">
            Add
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content>
      <ion-list>
        <ion-list-header><ion-label>YouTube link</ion-label></ion-list-header>
        <ion-item>
          <ion-textarea
            :value="text"
            :rows="3"
            :auto-grow="true"
            placeholder="Paste a video, playlist or channel link"
            data-testid="ytdl-url"
            @ionInput="text = String($event.target.value ?? '')"
          />
        </ion-item>
        <ion-item lines="none">
          <ion-button fill="clear" size="small" data-testid="ytdl-paste" @click="pasteFromClipboard">
            <ion-icon slot="start" :icon="clipboardOutline" />
            Paste
          </ion-button>
          <ion-note slot="end" data-testid="ytdl-count">
            {{ urls.length }} link{{ urls.length === 1 ? '' : 's' }} detected
          </ion-note>
        </ion-item>

        <ion-list-header><ion-label>Save as</ion-label></ion-list-header>
        <ion-item lines="none">
          <ion-segment
            :value="mode"
            data-testid="ytdl-mode"
            @ionChange="mode = ($event.detail.value ?? '') as 'music' | 'music-video' | ''"
          >
            <ion-segment-button value="music" data-testid="ytdl-mode-music">
              <ion-label>Music</ion-label>
            </ion-segment-button>
            <ion-segment-button value="music-video" data-testid="ytdl-mode-video">
              <ion-label>Music video</ion-label>
            </ion-segment-button>
          </ion-segment>
        </ion-item>
        <ion-item lines="none">
          <ion-note data-testid="ytdl-hint">
            <template v-if="mode === 'music'">
              Saved to your music library as audio, with cover art and lyrics.
            </template>
            <template v-else-if="mode === 'music-video'">
              Saved to your music video library at the best available quality.
            </template>
            <template v-else>
              Choose one. A playlist or channel link fetches everything in it
              that is a full-length track.
            </template>
          </ion-note>
        </ion-item>

        <ion-item v-if="progress" lines="none">
          <ion-note data-testid="ytdl-progress">{{ progress }}</ion-note>
        </ion-item>
        <ion-item v-if="error" lines="none">
          <ion-note color="danger" data-testid="ytdl-error">{{ error }}</ion-note>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-modal>
</template>

<style scoped>
/* Room to breathe (spec 1037, FR-008).
   Ionic's list items sit flush to the edge on a full-screen sheet, which on a
   phone puts text hard against the bezel. The inset is applied to the CONTENT
   rather than to each item so every row lines up, and the bottom clears the home
   indicator via the safe-area inset rather than a guessed constant. */
ion-content {
  --padding-start: 8px;
  --padding-end: 8px;
  --padding-top: 4px;
  --padding-bottom: calc(16px + var(--ion-safe-area-bottom, 0px));
}
</style>
