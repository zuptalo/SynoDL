<script setup lang="ts">
/**
 * One YouTube download in the Tasks list (spec 0012).
 *
 * Deliberately thinner than TaskItem: there is no progress bar, no percentage,
 * no speed and no estimate, because the server does not report them and the
 * absence is the requirement (FR-017). Row metrics match TaskItem exactly so a
 * mixed list does not make the eye jump.
 *
 * It also offers no pause or resume (FR-027). A worker is a running process on
 * a cluster, not a transfer we can steer — presenting a control we cannot
 * honour would be worse than presenting none.
 */
import { computed } from 'vue';
import {
  IonIcon,
  IonItem,
  IonItemOption,
  IonItemOptions,
  IonItemSliding,
  IonLabel,
} from '@ionic/vue';
import {
  checkmarkCircleOutline,
  hourglassOutline,
  musicalNotesOutline,
  trashOutline,
  videocamOutline,
  warningOutline,
} from 'ionicons/icons';
import type { YtdlDownload } from '@/services/api';

const props = defineProps<{ download: YtdlDownload }>();
const emit = defineEmits<{ (e: 'dismiss', requestId: string): void }>();

const isVideo = computed(() => props.download.mode === 'music-video');

// A YouTube link is not a title. Until the download finishes there is nothing
// better to show, so render the most identifying part of the URL rather than
// the whole query string.
const heading = computed(() => {
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

const stateLabel = computed(
  () =>
    ({
      scheduled: 'queued',
      started: 'downloading',
      completed: 'saved',
      failed: 'failed',
    })[props.download.state],
);

const stateColorVar = computed(
  () =>
    ({
      scheduled: 'var(--ion-color-medium)',
      started: 'var(--ion-color-primary)',
      completed: 'var(--ion-color-success)',
      failed: 'var(--ion-color-danger)',
    })[props.download.state],
);

const stateIcon = computed(
  () =>
    ({
      scheduled: hourglassOutline,
      started: isVideo.value ? videocamOutline : musicalNotesOutline,
      completed: checkmarkCircleOutline,
      failed: warningOutline,
    })[props.download.state],
);

const scopeLabel = computed(
  () => ({ single: '', playlist: 'playlist', channel: 'channel' })[props.download.scope],
);

// Only a finished download can be dismissed. Removing a running one would
// strand its worker mid-write, so the action is not offered at all.
const canDismiss = computed(
  () => props.download.state === 'completed' || props.download.state === 'failed',
);
</script>

<template>
  <ion-item-sliding :disabled="!canDismiss">
    <ion-item :detail="false" data-testid="ytdl-item">
      <div slot="start" class="poster" aria-hidden="true">
        <ion-icon :icon="stateIcon" class="poster-ph" :style="{ color: stateColorVar }" />
      </div>
      <ion-label>
        <h2 class="name" data-testid="ytdl-name">{{ heading }}</h2>
        <div class="media">
          <!-- The source marker (FR-026): a mixed list must say which system a
               row belongs to, since the two behave differently. -->
          <span class="type" data-testid="ytdl-source">YouTube</span>
          <span>{{ isVideo ? 'Music video' : 'Music' }}</span>
          <span v-if="scopeLabel">{{ scopeLabel }}</span>
        </div>
        <div class="meta">
          <span class="status" :style="{ color: stateColorVar }" data-testid="ytdl-status">
            {{ stateLabel }}
          </span>
          <span v-if="download.reason" class="reason" data-testid="ytdl-reason">
            {{ download.reason }}
          </span>
          <span v-if="download.submittedBy" class="added-by" data-testid="ytdl-added-by">
            added by {{ download.submittedBy }}
          </span>
        </div>
      </ion-label>
    </ion-item>
    <ion-item-options side="end">
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
.name {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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
.status {
  font-weight: 600;
}
.reason {
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
