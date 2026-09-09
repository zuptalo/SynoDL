<script setup lang="ts">
/**
 * Everything known about one upload (spec 1042).
 *
 * An upload was the only row in the Tasks list with nothing behind it. Since
 * spec 1040 it knows a track name, an artist and an album, and it has always
 * known where it was sent and how far along it is — none of which fitted on a
 * row.
 *
 * Deliberately the same shape as the YouTube download sheet: a hero image, then
 * label-and-value rows, then the timestamps. Two sheets describing two kinds of
 * transfer should not be two different designs.
 */
import { computed } from 'vue';
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
import type { UploadJob } from '@/composables/useUploads';
import { formatBytes, formatSpeed } from '@/utils/format';

const props = defineProps<{ isOpen: boolean; job: UploadJob | null }>();
defineEmits<{ (e: 'dismiss'): void }>();

const isMusic = computed(
  () => props.job?.kind === 'music' || props.job?.kind === 'music-video',
);

const kindLabel = computed(
  () =>
    ({ movie: 'Movie', tv: 'TV show', music: 'Music', 'music-video': 'Music video' })[
      props.job?.kind ?? 'movie'
    ] ?? '—',
);

const stateLabel = computed(
  () =>
    ({
      waiting: 'waiting its turn',
      sending: 'sending',
      done: 'uploaded',
      failed: 'failed',
      cancelled: 'stopped',
    })[props.job?.state ?? 'waiting'] ?? '',
);

const STATE_COLOR: Record<string, { fg: string; rgb: string; fallback: string }> = {
  waiting: { fg: 'var(--ion-color-medium)', rgb: '--ion-color-medium-rgb', fallback: '146, 148, 156' },
  sending: { fg: 'var(--ion-color-primary)', rgb: '--ion-color-primary-rgb', fallback: '16, 185, 129' },
  done: { fg: 'var(--ion-color-success)', rgb: '--ion-color-success-rgb', fallback: '45, 211, 111' },
  failed: { fg: 'var(--ion-color-danger)', rgb: '--ion-color-danger-rgb', fallback: '235, 68, 90' },
  cancelled: { fg: 'var(--ion-color-medium)', rgb: '--ion-color-medium-rgb', fallback: '146, 148, 156' },
};
const stateChipStyle = computed(() => {
  const c = STATE_COLOR[props.job?.state ?? 'waiting'] ?? STATE_COLOR.waiting;
  return { color: c.fg, background: `rgba(var(${c.rgb}, ${c.fallback}), 0.14)` };
});

const title = computed(() => {
  const j = props.job;
  if (!j) return '—';
  return (isMusic.value ? j.track : j.title) || j.name;
});

// Where it landed, which the server composed and reported back — so this is the
// real folder rather than the one the sheet guessed at.
const destination = computed(() => (props.job?.state === 'done' ? props.job.message : ''));

const sent = computed(() => {
  const j = props.job;
  if (!j) return '';
  return `${formatBytes(j.size - j.remaining)} of ${formatBytes(j.size)}`;
});
</script>

<template>
  <ion-modal :is-open="isOpen" @did-dismiss="$emit('dismiss')">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>Upload details</ion-title>
        <ion-buttons slot="end">
          <ion-button data-testid="upload-detail-close" @click="$emit('dismiss')">Close</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content>
      <!-- Dismissed while the sheet was open. The same honest answer a
           download's sheet gives rather than stale data. -->
      <div v-if="!job" class="gone" data-testid="upload-detail-gone">
        <p>This upload is no longer listed.</p>
      </div>
      <ion-list v-else data-testid="upload-detail">
        <ion-item v-if="job.artworkUrl">
          <img :src="job.artworkUrl" alt="" class="art" data-testid="upload-detail-artwork" />
        </ion-item>
        <ion-item>
          <ion-label class="ion-text-wrap">
            <p>{{ isMusic ? 'Track' : 'Title' }}</p>
            <h2 data-testid="upload-detail-title">{{ title }}</h2>
          </ion-label>
        </ion-item>
        <ion-item v-if="isMusic">
          <ion-label class="ion-text-wrap">
            <p>Artist</p>
            <h2 data-testid="upload-detail-artist">{{ job.artist || '—' }}</h2>
          </ion-label>
        </ion-item>
        <ion-item v-if="isMusic">
          <ion-label class="ion-text-wrap">
            <p>Album</p>
            <!-- Empty means Singles, which is where the server actually files
                 it — saying "—" here would describe a different folder. -->
            <h2 data-testid="upload-detail-album">{{ job.album || 'Singles' }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <p>Sent as</p>
            <h2 data-testid="upload-detail-kind">{{ kindLabel }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label class="ion-text-wrap">
            <p>File</p>
            <h2 data-testid="upload-detail-file">{{ job.name }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <p>State</p>
            <h2>
              <span class="state-chip" :style="stateChipStyle" data-testid="upload-detail-state">
                {{ stateLabel }}
              </span>
            </h2>
            <ion-progress-bar
              v-if="job.state === 'sending'"
              class="detail-bar"
              :value="job.progress"
            />
          </ion-label>
        </ion-item>
        <ion-item v-if="job.state === 'failed' || job.state === 'cancelled'">
          <ion-label class="ion-text-wrap">
            <p>Reason</p>
            <h2 data-testid="upload-detail-reason">{{ job.message || '—' }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <p>Progress</p>
            <h2 data-testid="upload-detail-progress">{{ sent }}</h2>
          </ion-label>
        </ion-item>
        <ion-item v-if="job.state === 'sending' && job.speed > 0">
          <ion-label>
            <p>Speed</p>
            <h2>↑ {{ formatSpeed(job.speed) }}</h2>
          </ion-label>
        </ion-item>
        <ion-item v-if="destination">
          <ion-label class="ion-text-wrap">
            <p>Landed in</p>
            <h2 data-testid="upload-detail-destination">{{ destination }}</h2>
          </ion-label>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-modal>
</template>

<style scoped>
.gone {
  display: flex;
  justify-content: center;
  padding-top: 20vh;
  color: var(--app-text-dim);
}
ion-label p {
  color: var(--app-text-dim);
  font-size: 0.72rem;
  text-transform: uppercase;
  letter-spacing: 0.03em;
}
ion-label h2 {
  font-size: 0.95rem;
}
.ion-text-wrap h2 {
  white-space: normal;
  word-break: break-word;
}
/* A 16:9 frame, held whatever the picked file's shape is, so the sheet does not
   reflow as the image loads. */
.art {
  width: 100%;
  aspect-ratio: 16 / 9;
  object-fit: cover;
  border-radius: 8px;
  display: block;
  background: rgba(var(--ion-text-color-rgb, 0, 0, 0), 0.08);
}
/* The same pill the row uses, so the two surfaces agree at a glance. */
.state-chip {
  display: inline-block;
  padding: 1px 8px;
  border-radius: 6px;
  font-weight: 600;
}
.detail-bar {
  margin-top: 8px;
  height: 6px;
  border-radius: 3px;
}
</style>
