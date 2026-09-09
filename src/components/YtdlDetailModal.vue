<script setup lang="ts">
/**
 * YouTube download detail sheet (spec 0013, US4).
 *
 * A stock Ionic modal, matching TaskDetailModal — deliberately, rather than the
 * first per-item ROUTE in a router that has none. A YouTube download sits in the
 * same list as a NAS task and should behave like one when you tap it.
 *
 * It is bound to the live download BY ID, so its fields update in place while
 * open: a progress bar that stops moving because the sheet is open would be
 * worse than no sheet.
 *
 * Every "unknown" here is rendered as an em dash rather than as a zero or an
 * empty row. A download that has not finished has no final timestamp, and
 * showing 1970 — or a bar at 0% — would be a confident wrong answer where the
 * honest one is "not yet" (FR-033).
 */
import {
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonItem,
  IonLabel,
  IonList,
  IonModal,
  IonIcon,
  IonProgressBar,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { copyOutline } from 'ionicons/icons';
import { computed, onUnmounted, ref, watch } from 'vue';
import { api, type YtdlDownload } from '@/services/api';
import { onYtdlUpdate } from '@/composables/useYtdl';
import { appToast } from '@/services/toast';
import { formatTimestamp } from '@/utils/format';

/**
 * `download` is the row from the Tasks list, when there is one — a single
 * download or a group. It is live: the list re-reads on its own schedule.
 *
 * An ITEM of a group has no row in that list (they are excluded so an expanded
 * channel cannot crowd it out), so the sheet fetches it by id instead. Without
 * that, tapping any item showed "This download is no longer available",
 * whatever state it was in.
 */
const props = defineProps<{
  isOpen: boolean;
  requestId: string | null;
  download: YtdlDownload | null;
}>();
defineEmits<{ (e: 'dismiss'): void; (e: 'retry', requestId: string): void }>();

// Retry is offered here as well as on the row, because the sheet is where
// someone works out WHY it failed — and having decided, they should not have to
// close it and find the row again (FR-026, FR-028).
const canRetry = computed(() => resolved.value?.state === 'failed');

// What the sheet is showing: the live row when the list has one, otherwise
// whatever was fetched by id.
const fetched = ref<YtdlDownload | null>(null);
const notFound = ref(false);
const resolved = computed<YtdlDownload | null>(() => props.download ?? fetched.value);

async function loadById(): Promise<void> {
  const id = props.requestId;
  if (!id || props.download) return;
  try {
    fetched.value = await api.ytdlOne(id);
    notFound.value = false;
  } catch {
    // A download that really is gone — dismissed elsewhere, or never existed.
    notFound.value = true;
  }
}

/**
 * A fetched row follows the live stream rather than re-asking on a timer
 * (spec 1038).
 *
 * It used to re-request itself every five seconds, because a row fetched by id
 * is not in the polled list and nothing else would have moved its progress bar.
 * The stream carries it now, so the sheet listens instead of asks — and an item
 * inside a playlist, which is exactly the case that has to be fetched by id, is
 * the case that benefits most.
 *
 * A row the LIST already holds is untouched here: it is live by definition, and
 * asking for it again would be a second request for something already at hand.
 */
const stopListening = onYtdlUpdate((update) => {
  const id = props.requestId;
  if (!props.isOpen || !id || props.download) return;
  if ((update.removed ?? []).includes(id)) {
    // Dismissed while the sheet was open. This is the one case the sheet's gone
    // state is actually for.
    fetched.value = null;
    notFound.value = true;
    return;
  }
  for (const d of [...(update.changed ?? []), ...(update.created ?? [])]) {
    if (d.requestId === id) {
      fetched.value = d;
      notFound.value = false;
      return;
    }
  }
});
onUnmounted(stopListening);

watch(
  [() => props.isOpen, () => props.requestId],
  ([open]) => {
    fetched.value = null;
    notFound.value = false;
    if (!open) return;
    void loadById();
  },
  { immediate: true },
);

/**
 * Copy the link (FR-005). Matches how a NAS task's source link already behaves,
 * so the same gesture works wherever a link is shown.
 *
 * A refusal is reported rather than swallowed: a copy that silently did nothing
 * is worse than one that says it could not, because the reader only finds out
 * when they paste (FR-006).
 */
async function copyLink(): Promise<void> {
  const url = resolved.value?.url;
  if (!url) return;
  try {
    await navigator.clipboard.writeText(url);
    await appToast({ message: 'Link copied.', duration: 1600 });
  } catch {
    await appToast({ message: 'Could not copy the link.', color: 'danger', duration: 2200 });
  }
}

const isVideo = computed(() => resolved.value?.mode === 'music-video');

const stateLabel = computed(() =>
  resolved.value
    ? {
        resolving: 'reading contents',
        queued: 'waiting its turn',
        scheduled: 'starting',
        downloading: 'downloading',
        completed: 'saved',
        failed: 'failed',
      }[resolved.value.state]
    : '',
);

const scopeLabel = computed(() =>
  resolved.value
    ? { single: 'One video', playlist: 'Playlist', channel: 'Channel' }[resolved.value.scope]
    : '',
);

// Present only while running AND only when something is genuinely known.
const progress = computed(() =>
  resolved.value?.state === 'downloading' && resolved.value.progress !== undefined
    ? resolved.value.progress
    : undefined,
);
const percentLabel = computed(() =>
  progress.value === undefined ? '—' : `${Math.floor(progress.value * 100)}%`,
);

/**
 * Lyrics are a three-way answer, not a boolean: saved, not published by the
 * source, or not yet known because the download has not finished. Collapsing
 * the last two into "no lyrics" would tell a user their track has none while it
 * is still being fetched.
 */
const lyricsLabel = computed(() => {
  const d = resolved.value;
  if (!d) return '—';
  if (d.hasLyrics) return d.lyricsLang ? `Saved (${d.lyricsLang})` : 'Saved';
  if (d.state === 'completed') return 'None published';
  return '—';
});

const artworkSrc = computed(() =>
  resolved.value?.artwork ? `/v1/ytdl/thumb?u=${encodeURIComponent(resolved.value.artwork)}` : '',
);

// Above 1 means it has been tried again, which is worth saying plainly.
const attemptsLabel = computed(() => {
  const n = resolved.value?.attempts ?? 1;
  return n > 1 ? `${n} attempts` : '';
});
</script>

<template>
  <ion-modal :is-open="isOpen" @did-dismiss="$emit('dismiss')">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>Download details</ion-title>
        <ion-buttons slot="start">
          <ion-button
            v-if="canRetry && resolved"
            data-testid="ytdl-detail-retry"
            @click="$emit('retry', resolved.requestId)"
          >
            Retry
          </ion-button>
        </ion-buttons>
        <ion-buttons slot="end">
          <ion-button data-testid="ytdl-detail-close" @click="$emit('dismiss')">Close</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content>
      <!-- Only once a lookup has actually concluded: while a fetched row is in
           flight there is nothing yet, and saying it is gone would be wrong. -->
      <div v-if="!resolved && notFound" class="gone" data-testid="ytdl-detail-gone">
        <p>This download is no longer available.</p>
      </div>
      <div v-else-if="!resolved" class="gone"><p>Loading…</p></div>
      <ion-list v-else data-testid="ytdl-detail">
        <ion-item v-if="artworkSrc">
          <img :src="artworkSrc" alt="" class="art" data-testid="ytdl-detail-artwork" />
        </ion-item>
        <ion-item>
          <ion-label class="ion-text-wrap">
            <p>Title</p>
            <h2 data-testid="ytdl-detail-title">{{ resolved.title || '—' }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label class="ion-text-wrap">
            <p>Artist</p>
            <h2 data-testid="ytdl-detail-uploader">{{ resolved.uploader || '—' }}</h2>
          </ion-label>
        </ion-item>
        <!-- Only an expanded item has a group, and this is the one place it is
             visible when the item is viewed on its own (FR-019c). -->
        <ion-item v-if="resolved.groupName">
          <ion-label class="ion-text-wrap">
            <p>From</p>
            <h2 data-testid="ytdl-detail-group">{{ resolved.groupName }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <p>Saved as</p>
            <h2 data-testid="ytdl-detail-mode">{{ isVideo ? 'Music video' : 'Music' }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <p>Kind</p>
            <h2 data-testid="ytdl-detail-scope">{{ scopeLabel }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <p>State</p>
            <h2 data-testid="ytdl-detail-state">{{ stateLabel }}</h2>
            <ion-progress-bar v-if="progress !== undefined" :value="progress" />
          </ion-label>
        </ion-item>
        <ion-item v-if="resolved.state === 'downloading'">
          <ion-label>
            <p>Progress</p>
            <h2 data-testid="ytdl-detail-progress">{{ percentLabel }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <p>Lyrics</p>
            <h2 data-testid="ytdl-detail-lyrics">{{ lyricsLabel }}</h2>
          </ion-label>
        </ion-item>
        <ion-item v-if="resolved.reason">
          <ion-label class="ion-text-wrap">
            <p>Reason</p>
            <h2 data-testid="ytdl-detail-reason">{{ resolved.reason }}</h2>
          </ion-label>
        </ion-item>
        <ion-item v-if="attemptsLabel">
          <ion-label>
            <p>Attempts</p>
            <h2 data-testid="ytdl-detail-attempts">{{ attemptsLabel }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label class="ion-text-wrap">
            <p>Created</p>
            <h2 data-testid="ytdl-detail-created">{{ formatTimestamp(resolved.submittedAt) }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label class="ion-text-wrap">
            <p>Finished</p>
            <h2 data-testid="ytdl-detail-finished">{{ formatTimestamp(resolved.finishedAt) }}</h2>
          </ion-label>
        </ion-item>
        <ion-item v-if="resolved.submittedBy">
          <ion-label class="ion-text-wrap">
            <p>Added by</p>
            <h2 data-testid="ytdl-detail-added-by">{{ resolved.submittedBy }}</h2>
          </ion-label>
        </ion-item>
        <ion-item button :detail="false" data-testid="ytdl-detail-url-row" @click="copyLink">
          <ion-label class="ion-text-wrap">
            <p>Link</p>
            <h2 class="link" data-testid="ytdl-detail-url">{{ resolved.url }}</h2>
          </ion-label>
          <ion-icon slot="end" :icon="copyOutline" aria-hidden="true" />
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

.gone {
  padding: 24px;
  text-align: center;
  color: var(--ion-color-medium);
}
/* 16:9, shown whole rather than cover-cropped: here there is room for it, and
   the point of the sheet is to show what the thing IS. */
.art {
  width: 100%;
  border-radius: 8px;
  display: block;
}
.link {
  overflow-wrap: anywhere;
  font-size: 0.9rem;
}
ion-progress-bar {
  margin-top: 6px;
  height: 3px;
  border-radius: 2px;
}
</style>
