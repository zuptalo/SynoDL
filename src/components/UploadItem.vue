<script setup lang="ts">
/**
 * An upload rendered as a first-class task row.
 *
 * Deliberately the same shape as TaskItem — thumbnail, title, media line, status
 * line, thin progress bar, swipe actions — because an upload IS a transfer in
 * progress and reading as a different kind of object made it look like a
 * transient notice rather than work the NAS is doing. The only differences are
 * the ones that are real: the arrow points up, and the actions are retry and
 * replace rather than pause and resume.
 */
import {
  IonIcon,
  IonItem,
  IonItemOption,
  IonItemOptions,
  IonItemSliding,
  IonLabel,
  IonProgressBar,
} from '@ionic/vue';
import {
  cloudUploadOutline,
  closeOutline,
  refreshOutline,
  swapHorizontalOutline,
} from 'ionicons/icons';
import { computed, ref } from 'vue';
import type { UploadJob } from '@/composables/useUploads';
import { formatBytes, formatEta, formatPercent, formatSpeed } from '@/utils/format';
import { splitYear } from '@/services/title-year';

const props = defineProps<{ job: UploadJob }>();
const emit = defineEmits<{
  (e: 'open', id: number): void;
  (e: 'stop', id: number): void;
  (e: 'retry', id: number, overwrite?: boolean): void;
  (e: 'clear', id: number): void;
}>();

// The title the user typed, not the raw file name: "Coyote vs. Acme 2026" reads
// as a title, "Coyote_vs_Acme_2026_1080p_WEBRip_AOC_30NAMA.mkv" does not.
const heading = computed(() => splitYear(props.job.title));

// Music describes itself differently from a film: a track, an artist and an
// album rather than one title and a year (spec 1040). The row follows suit
// rather than showing a track's file name under a "Title" heading.
const isMusic = computed(
  () => props.job.kind === 'music' || props.job.kind === 'music-video',
);
const rowTitle = computed(() =>
  isMusic.value ? props.job.track || props.job.name : heading.value.title,
);
/** The facts under the title, in the order a reader looks for them. */
const rowFacts = computed(() => {
  if (!isMusic.value) return heading.value.year ? [heading.value.year] : [];
  return [props.job.artist, props.job.album].filter((x) => x !== '');
});
const kindLabel = computed(
  () =>
    ({ movie: 'Movie', tv: 'TV show', music: 'Music', 'music-video': 'Music video' })[
      props.job.kind
    ] ?? '',
);

// Artwork the reader picked, rendered straight off the device — no request, and
// nothing read back off the NAS to draw a row (FR-010). A file the browser turns
// out not to be able to show falls back to the icon rather than a hole.
const artFailed = ref(false);
const artworkSrc = computed(() => (artFailed.value ? '' : props.job.artworkUrl));

// The same chip every other row uses (spec 1039), so an upload reads like the
// downloads beside it rather than like a notice.
const STATE_TINT: Record<string, { rgb: string; fallback: string }> = {
  'var(--ion-color-primary)': { rgb: '--ion-color-primary-rgb', fallback: '16, 185, 129' },
  'var(--ion-color-success)': { rgb: '--ion-color-success-rgb', fallback: '45, 211, 111' },
  'var(--ion-color-danger)': { rgb: '--ion-color-danger-rgb', fallback: '235, 68, 90' },
  'var(--ion-color-medium)': { rgb: '--ion-color-medium-rgb', fallback: '146, 148, 156' },
};
const stateChipStyle = computed(() => {
  const fg = statusColorVar.value;
  const tint = STATE_TINT[fg] ?? STATE_TINT['var(--ion-color-medium)'];
  return { color: fg, background: `rgba(var(${tint.rgb}, ${tint.fallback}), 0.14)` };
});
const active = computed(() => props.job.state === 'sending');
/**
 * The device has sent everything, but the NAS has not confirmed the write.
 *
 * Upload progress only measures the FIRST hop — browser to synodl. The second
 * hop, synodl to DSM, is invisible to it, so the bar reaches 100% while real
 * work is still outstanding. Showing a full bar there claims a completion
 * nothing has observed, so this stretch is reported as its own step.
 */
const finishing = computed(() => props.job.state === 'sending' && props.job.progress >= 1);
const finished = computed(
  () => props.job.state === 'done' || props.job.state === 'failed' || props.job.state === 'cancelled',
);

const label = computed(() => {
  switch (props.job.state) {
    case 'waiting':
      return 'Queued';
    case 'sending':
      return finishing.value ? 'Finishing' : 'Uploading';
    case 'done':
      return 'Uploaded';
    case 'cancelled':
      return 'Stopped';
    default:
      return 'Failed';
  }
});

// Mirrors how TaskItem colours a status, so the two lists read as one thing.
const statusColorVar = computed(() => {
  switch (props.job.state) {
    case 'done':
      return 'var(--app-status-finished)';
    case 'failed':
      return 'var(--app-status-error)';
    case 'sending':
      return 'var(--ion-color-primary)';
    default:
      return 'var(--app-text-dim)';
  }
});
</script>

<template>
  <ion-item-sliding>
    <ion-item button :detail="false" data-testid="upload-item" @click="emit('open', job.id)">
      <div slot="start" class="poster" aria-hidden="true">
        <img
          v-if="artworkSrc"
          :src="artworkSrc"
          alt=""
          data-testid="upload-artwork"
          @error="artFailed = true"
        />
        <ion-icon v-else :icon="cloudUploadOutline" class="poster-ph" />
      </div>
      <ion-label>
        <h2 class="name" data-testid="upload-name">{{ rowTitle }}</h2>
        <div v-if="kindLabel || rowFacts.length" class="media">
          <span v-if="kindLabel" class="type">{{ kindLabel }}</span>
          <span v-for="f in rowFacts" :key="f" data-testid="upload-fact">{{ f }}</span>
        </div>
        <div class="meta">
          <span class="status" :style="stateChipStyle" data-testid="upload-status">
            {{ label }}
          </span>
          <span v-if="job.state === 'failed' || job.state === 'cancelled'" class="reason">
            {{ job.message }}
          </span>
          <template v-else-if="finishing">
            <span>{{ formatBytes(job.size) }} sent — saving on the NAS…</span>
          </template>
          <template v-else>
            <span>{{ formatPercent(job.size - job.remaining, job.size) }} of {{ formatBytes(job.size) }}</span>
            <span v-if="active && job.speed > 0">↑ {{ formatSpeed(job.speed) }}</span>
            <span v-if="active && job.speed > 0">{{ formatEta(job.remaining, job.speed) }}</span>
          </template>
        </div>
        <p v-if="job.state === 'done'" class="dest" data-testid="upload-result">{{ job.message }}</p>
        <!-- Indeterminate once the device is done: the remaining work is real
             but its duration is unknown to us, and a bar pinned at 100% would
             read as finished while the NAS is still writing. -->
        <div class="bar-slot">
          <ion-progress-bar
            v-if="job.state !== 'done'"
            :type="finishing ? 'indeterminate' : 'determinate'"
            :value="job.progress"
            :style="{ '--progress-background': statusColorVar }"
            data-testid="upload-progress"
          />
        </div>
      </ion-label>
    </ion-item>

    <ion-item-options side="end">
      <ion-item-option v-if="!finished" color="medium" data-testid="upload-stop" @click="emit('stop', job.id)">
        <ion-icon slot="icon-only" :icon="closeOutline" />
      </ion-item-option>
      <ion-item-option
        v-if="job.state === 'failed' || job.state === 'cancelled'"
        data-testid="upload-retry"
        @click="emit('retry', job.id)"
      >
        <ion-icon slot="icon-only" :icon="refreshOutline" />
      </ion-item-option>
      <!-- Replacing destroys what is on the NAS, so it is offered only for a real
           name collision — the case where a partial file blocks the retry. -->
      <ion-item-option
        v-if="job.replaceable"
        color="danger"
        data-testid="upload-replace"
        @click="emit('retry', job.id, true)"
      >
        <ion-icon slot="icon-only" :icon="swapHorizontalOutline" />
      </ion-item-option>
      <ion-item-option v-if="finished" color="medium" data-testid="upload-clear" @click="emit('clear', job.id)">
        <ion-icon slot="icon-only" :icon="closeOutline" />
      </ion-item-option>
    </ion-item-options>
  </ion-item-sliding>
</template>

<style scoped>
/* Matches TaskItem's row metrics exactly; an upload sitting above a download
   should not shift the eye. */
.poster {
  width: 40px;
  height: 60px;
  margin-inline-end: 12px;
  border-radius: 6px;
  overflow: hidden;
  flex-shrink: 0;
  background: rgba(var(--ion-text-color-rgb, 0, 0, 0), 0.08);
  display: flex;
  align-items: center;
  justify-content: center;
}
.poster-ph {
  font-size: 20px;
  color: var(--app-text-dim);
}
.name {
  font-size: 0.95rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.media {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.75rem;
  color: var(--app-text-dim);
  margin-top: 0.15rem;
}
.meta {
  display: flex;
  gap: 0.75rem;
  font-size: 0.78rem;
  color: var(--app-text-dim);
  margin: 0.2rem 0 0.35rem;
  /* One line, always. The speed and ETA change width on every poll, and letting
     the row reflow made it flip between one and two lines several times a
     minute — the row visibly jumping while nothing meaningful had changed.
     Nothing wraps; the least important item gives way instead. */
  flex-wrap: nowrap;
  white-space: nowrap;
  overflow: hidden;
}
.meta > * {
  /* Hold their natural width so a number changing digits cannot resize them. */
  flex: 0 0 auto;
}
.reason {
  /* A failure message is free text and can be long; it gives way rather than
     forcing the row to a second line. */
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.reason {
  color: var(--app-status-error);
}
.dest {
  font-size: 0.75rem;
  color: var(--app-text-dim);
}
/* The poster slot matches every other row's, so a mixed list keeps one left
   edge whatever kind of thing is in it. */
.poster img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
/* The established chip: same padding, radius and weight as the media-type pill
   on a NAS row and the state pill on a YouTube one. */
.status {
  padding: 1px 6px;
  border-radius: 6px;
  font-weight: 600;
}
.media .type {
  padding: 1px 6px;
  border-radius: 6px;
  font-weight: 600;
  color: var(--ion-color-primary);
  background: rgba(var(--ion-color-primary-rgb, 16, 185, 129), 0.14);
}
/* Reserved whether or not the bar is in it, so a row does not shrink the
   moment its upload finishes. Same slot as TaskItem and YtdlItem. */
.bar-slot {
  height: 3px;
}
ion-progress-bar {
  height: 3px;
  border-radius: 2px;
}
</style>
