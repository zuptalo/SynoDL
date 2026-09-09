<script setup lang="ts">
/**
 * One YouTube download in the Tasks list (spec 0012, extended by spec 0013).
 *
 * It now carries a progress bar, which spec 0012 deliberately did not: back then
 * the worker had no channel back to SynoDL, so any percentage would have been
 * invented. The server reads the worker's own output now, and reports a
 * fraction only while a download is actually running and only when something is
 * genuinely known — so an ABSENT progress value means "we cannot see", which is
 * rendered as no bar rather than as a bar at zero.
 *
 * Row metrics still match TaskItem exactly so a mixed list does not make the eye
 * jump, and there is still no speed and no estimate: nothing produces them.
 *
 * It also offers no pause or resume (FR-027). A worker is a running process on
 * a cluster, not a transfer we can steer — presenting a control we cannot
 * honour would be worse than presenting none.
 */
import { computed, ref } from 'vue';
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
  checkmarkCircleOutline,
  hourglassOutline,
  musicalNotesOutline,
  refreshOutline,
  trashOutline,
  videocamOutline,
  warningOutline,
} from 'ionicons/icons';
import type { YtdlDownload } from '@/services/api';
import { ytdlThumbSrc } from '@/services/ytdl-thumb';

const props = defineProps<{ download: YtdlDownload }>();
const emit = defineEmits<{
  (e: 'dismiss', requestId: string): void;
  (e: 'open', requestId: string): void;
  (e: 'retry', requestId: string): void;
}>();

const isVideo = computed(() => props.download.mode === 'music-video');

// The item's own title when the source would tell us (spec 1034), falling back
// to the most identifying part of the link. The fallback is not a rare path: a
// channel publishes no metadata document at all.
const heading = computed(() => props.download.title || linkLabel.value);

// The readable form of the link, for when there is no title to show.
const linkLabel = computed(() => {
  const d = props.download;
  try {
    const u = new URL(d.url);
    if (u.hostname === 'youtu.be') return `youtu.be/${u.pathname.replace(/^\//, '')}`;
    if (u.pathname.startsWith('/watch')) return `youtube.com/watch?v=${u.searchParams.get('v') ?? ''}`;
    if (u.pathname === '/playlist') return `Playlist ${u.searchParams.get('list') ?? ''}`;
    return `youtube.com${u.pathname}`;
  } catch {
    return d.url;
  }
});

// Six states (spec 0013). `queued` and `scheduled` read differently on purpose:
// "waiting its turn" is SynoDL holding it behind the parallel limit, which can
// be a long wait and is nobody's fault, while "starting" means the worker is
// coming up now. Collapsing them would make the first look like the second was
// hanging.
const stateLabel = computed(
  () =>
    ({
      resolving: 'reading contents',
      queued: 'waiting its turn',
      scheduled: 'starting',
      downloading: 'downloading',
      completed: 'saved',
      failed: 'failed',
    })[props.download.state],
);

// Each state's Ionic colour, as BOTH the colour token and its rgb triple. The
// triple is what lets the chip tint its background from the same colour the text
// uses — which is exactly how the media-type chip on a NAS row is built, and the
// reason this reads as the same component rather than a second invention
// (spec 1039, FR-005).
const STATE_COLOR: Record<string, { fg: string; rgb: string; fallback: string }> = {
  resolving: { fg: 'var(--ion-color-medium)', rgb: '--ion-color-medium-rgb', fallback: '146, 148, 156' },
  queued: { fg: 'var(--ion-color-medium)', rgb: '--ion-color-medium-rgb', fallback: '146, 148, 156' },
  scheduled: { fg: 'var(--ion-color-medium)', rgb: '--ion-color-medium-rgb', fallback: '146, 148, 156' },
  downloading: { fg: 'var(--ion-color-primary)', rgb: '--ion-color-primary-rgb', fallback: '16, 185, 129' },
  completed: { fg: 'var(--ion-color-success)', rgb: '--ion-color-success-rgb', fallback: '45, 211, 111' },
  failed: { fg: 'var(--ion-color-danger)', rgb: '--ion-color-danger-rgb', fallback: '235, 68, 90' },
};

const stateColor = computed(() => STATE_COLOR[props.download.state] ?? STATE_COLOR.queued);
const stateColorVar = computed(() => stateColor.value.fg);

// The chip's own two custom properties, set inline because the colour depends on
// the state rather than on a class. Alpha 0.14 is the established tint.
const stateChipStyle = computed(() => ({
  color: stateColor.value.fg,
  background: `rgba(var(${stateColor.value.rgb}, ${stateColor.value.fallback}), 0.14)`,
}));

const stateIcon = computed(
  () =>
    ({
      resolving: hourglassOutline,
      queued: hourglassOutline,
      scheduled: hourglassOutline,
      downloading: isVideo.value ? videocamOutline : musicalNotesOutline,
      completed: checkmarkCircleOutline,
      failed: warningOutline,
    })[props.download.state],
);

const scopeLabel = computed(
  () => ({ single: '', playlist: 'playlist', channel: 'channel' })[props.download.scope],
);

// Artwork goes through the server so the viewer's browser never contacts
// Google (spec 1034, FR-008). Its own proxy, not the catalog poster one: those
// hosts come from the download sources, and YouTube is not one of them.
//
// Asked for at the 16:9 size (spec 1039). Artwork is STORED as `hqdefault`,
// which is a 4:3 frame with black bands above and below a 16:9 image — so
// cropping it into this 40×60 slot kept the bands, and the tiles read as dark
// slivers next to a film poster that fills its slot. Same frame, right shape.
const artworkSrc = computed(() => ytdlThumbSrc(props.download.artwork, 'mq'));
// A thumbnail that 404s must leave the icon behind, not a hole (FR-007).
const artworkFailed = ref(false);

// A percentage only exists while a worker is running and only when its output
// could be read. Absent is a real answer here — see the file comment — so it is
// checked with `!== undefined` rather than truthiness, or a genuine 0% would be
// indistinguishable from "unknown".
const progress = computed(() =>
  props.download.state === 'downloading' && props.download.progress !== undefined
    ? props.download.progress
    : undefined,
);
const percentLabel = computed(() =>
  progress.value === undefined ? '' : `${Math.floor(progress.value * 100)}%`,
);

// A group reports how its items are getting on rather than a percentage of its
// own — "38 of 340 saved" says more than a bar (FR-019).
const groupSummary = computed(() => {
  const c = props.download.counts;
  if (!c) return '';
  const parts = [`${c.completed} of ${c.total} saved`];
  if (c.failed > 0) parts.push(`${c.failed} failed`);
  return parts.join(' · ');
});

// Retry is offered only for a failed download (FR-028): retrying a completed
// one would re-download what is already saved, and a running one has nothing to
// recover from yet.
const canRetry = computed(() => props.download.state === 'failed');

// Any download can be dismissed (spec 0013, FR-005c). Spec 0012 offered this
// only on a finished one, because dismissing a running download would have
// stranded its worker. It no longer does: the record goes at once and the worker
// is left to finish, so there is no reason to make someone wait out a channel
// they started by mistake.
const canDismiss = computed(() => true);
</script>

<template>
  <ion-item-sliding :disabled="!canDismiss">
    <ion-item
      button
      :detail="false"
      data-testid="ytdl-item"
      @click="emit('open', download.requestId)"
    >
      <div slot="start" class="poster" aria-hidden="true">
        <img
          v-if="artworkSrc && !artworkFailed"
          :src="artworkSrc"
          alt=""
          loading="lazy"
          data-testid="ytdl-artwork"
          @error="artworkFailed = true"
        />
        <ion-icon v-else :icon="stateIcon" class="poster-ph" :style="{ color: stateColorVar }" />
      </div>
      <ion-label>
        <h2 class="name" data-testid="ytdl-name">{{ heading }}</h2>
        <div class="media">
          <!-- The source marker (FR-026): a mixed list must say which system a
               row belongs to, since the two behave differently. -->
          <span class="type" data-testid="ytdl-source">YouTube</span>
          <span v-if="download.uploader" class="uploader" data-testid="ytdl-uploader">
            {{ download.uploader }}
          </span>
          <span>{{ isVideo ? 'Music video' : 'Music' }}</span>
          <span v-if="scopeLabel">{{ scopeLabel }}</span>
        </div>
        <div class="meta">
          <!-- A chip rather than a coloured word (FR-004): on an expanded
               playlist of a few dozen tracks, telling which one is running
               otherwise means reading every row. Same pill the NAS row uses for
               its media type — the idiom the list already has. -->
          <span class="status" :style="stateChipStyle" data-testid="ytdl-status">
            {{ stateLabel }}
          </span>
          <span v-if="percentLabel" data-testid="ytdl-percent">{{ percentLabel }}</span>
          <span v-if="groupSummary" data-testid="ytdl-group-summary">{{ groupSummary }}</span>
          <span v-if="download.reason" class="reason" data-testid="ytdl-reason">
            {{ download.reason }}
          </span>
          <span v-if="download.submittedBy" class="added-by" data-testid="ytdl-added-by">
            added by {{ download.submittedBy }}
          </span>
        </div>
        <!-- No bar at all when nothing is known: a bar pinned at zero reads as a
             stalled download, which is exactly the wrong thing to say.

             The SPACE it would occupy is reserved even so. A bar that appears
             and disappears as a download starts and finishes otherwise makes
             every row below it jump, which is most visible on an expanded
             playlist where items finish one after another under the eye. The
             slot is the bar's own height, so a row measures the same whether
             the bar is in it or not. -->
        <div class="bar-slot">
          <ion-progress-bar
            v-if="progress !== undefined"
            :value="progress"
            data-testid="ytdl-progress"
            :style="{ '--progress-background': stateColorVar }"
          />
        </div>
      </ion-label>
    </ion-item>
    <ion-item-options side="end">
      <ion-item-option
        v-if="canRetry"
        color="success"
        data-testid="ytdl-retry"
        @click="emit('retry', download.requestId)"
      >
        <ion-icon slot="icon-only" :icon="refreshOutline" />
      </ion-item-option>
      <ion-item-option
        v-if="canDismiss"
        color="danger"
        data-testid="ytdl-dismiss"
        @click="emit('dismiss', download.requestId)"
      >
        <ion-icon slot="icon-only" :icon="trashOutline" />
      </ion-item-option>
    </ion-item-options>
  </ion-item-sliding>
</template>

<style scoped>
/* Matches TaskItem's row metrics exactly; a YouTube download sitting among
   NAS downloads should not shift the eye. */
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
  font-size: 22px;
}
/* A YouTube thumbnail is 16:9 and the slot is portrait, so cover-crop it rather
   than letterboxing — the row's metrics match TaskItem's and must not shift. */
.poster img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
/* A published title can be long and arrives in any script, so it is clipped to
   one line and the row keeps the height of every other row in the list,
   whatever the track is called (FR-011). */
.name {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.uploader {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 40%;
}
.media,
.meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 0.8rem;
  color: var(--ion-color-medium);
}
.media {
  margin-top: 2px;
}
.type {
  text-transform: uppercase;
  letter-spacing: 0.04em;
  font-weight: 600;
}
/* The established chip: same padding, radius, weight and tint alpha as the
   media-type pill on a NAS row. Only the colour differs, and it comes from the
   state. */
.status {
  padding: 1px 6px;
  border-radius: 6px;
  font-weight: 600;
}

/* Matches TaskItem's bar exactly, for the same reason the row metrics do —
   including the reserved slot, so a mixed list stays flush whichever kind of
   row is currently showing a bar. */
.bar-slot {
  height: 3px;
}
ion-progress-bar {
  height: 3px;
  border-radius: 2px;
}
.reason {
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
