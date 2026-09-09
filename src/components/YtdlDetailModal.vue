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
  IonProgressBar,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { computed } from 'vue';
import type { YtdlDownload } from '@/services/api';
import { formatTimestamp } from '@/utils/format';

const props = defineProps<{ isOpen: boolean; download: YtdlDownload | null }>();
defineEmits<{ (e: 'dismiss'): void; (e: 'retry', requestId: string): void }>();

// Retry is offered here as well as on the row, because the sheet is where
// someone works out WHY it failed — and having decided, they should not have to
// close it and find the row again (FR-026, FR-028).
const canRetry = computed(() => props.download?.state === 'failed');

const isVideo = computed(() => props.download?.mode === 'music-video');

const stateLabel = computed(() =>
  props.download
    ? {
        resolving: 'reading contents',
        queued: 'waiting its turn',
        scheduled: 'starting',
        downloading: 'downloading',
        completed: 'saved',
        failed: 'failed',
      }[props.download.state]
    : '',
);

const scopeLabel = computed(() =>
  props.download
    ? { single: 'One video', playlist: 'Playlist', channel: 'Channel' }[props.download.scope]
    : '',
);

// Present only while running AND only when something is genuinely known.
const progress = computed(() =>
  props.download?.state === 'downloading' && props.download.progress !== undefined
    ? props.download.progress
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
  const d = props.download;
  if (!d) return '—';
  if (d.hasLyrics) return d.lyricsLang ? `Saved (${d.lyricsLang})` : 'Saved';
  if (d.state === 'completed') return 'None published';
  return '—';
});

const artworkSrc = computed(() =>
  props.download?.artwork ? `/v1/ytdl/thumb?u=${encodeURIComponent(props.download.artwork)}` : '',
);

// Above 1 means it has been tried again, which is worth saying plainly.
const attemptsLabel = computed(() => {
  const n = props.download?.attempts ?? 1;
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
            v-if="canRetry && download"
            data-testid="ytdl-detail-retry"
            @click="$emit('retry', download.requestId)"
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
      <div v-if="!download" class="gone" data-testid="ytdl-detail-gone">
        <p>This download is no longer available.</p>
      </div>
      <ion-list v-else data-testid="ytdl-detail">
        <ion-item v-if="artworkSrc">
          <img :src="artworkSrc" alt="" class="art" data-testid="ytdl-detail-artwork" />
        </ion-item>
        <ion-item>
          <ion-label class="ion-text-wrap">
            <p>Title</p>
            <h2 data-testid="ytdl-detail-title">{{ download.title || '—' }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label class="ion-text-wrap">
            <p>Artist</p>
            <h2 data-testid="ytdl-detail-uploader">{{ download.uploader || '—' }}</h2>
          </ion-label>
        </ion-item>
        <!-- Only an expanded item has a group, and this is the one place it is
             visible when the item is viewed on its own (FR-019c). -->
        <ion-item v-if="download.groupName">
          <ion-label class="ion-text-wrap">
            <p>From</p>
            <h2 data-testid="ytdl-detail-group">{{ download.groupName }}</h2>
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
        <ion-item v-if="download.state === 'downloading'">
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
        <ion-item v-if="download.reason">
          <ion-label class="ion-text-wrap">
            <p>Reason</p>
            <h2 data-testid="ytdl-detail-reason">{{ download.reason }}</h2>
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
            <h2 data-testid="ytdl-detail-created">{{ formatTimestamp(download.submittedAt) }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label class="ion-text-wrap">
            <p>Finished</p>
            <h2 data-testid="ytdl-detail-finished">{{ formatTimestamp(download.finishedAt) }}</h2>
          </ion-label>
        </ion-item>
        <ion-item v-if="download.submittedBy">
          <ion-label class="ion-text-wrap">
            <p>Added by</p>
            <h2 data-testid="ytdl-detail-added-by">{{ download.submittedBy }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label class="ion-text-wrap">
            <p>Link</p>
            <h2 class="link" data-testid="ytdl-detail-url">{{ download.url }}</h2>
          </ion-label>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-modal>
</template>

<style scoped>
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
