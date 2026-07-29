<script setup lang="ts">
import {
  IonCheckbox,
  IonIcon,
  IonItem,
  IonItemOption,
  IonItemOptions,
  IonItemSliding,
  IonLabel,
  IonProgressBar,
} from '@ionic/vue';
import { pauseOutline, playOutline, trashOutline } from 'ionicons/icons';
import { computed, ref } from 'vue';
import type { Task } from '@/types/task';
import { formatBytes, formatEta, formatPercent, formatSpeed, progressOf } from '@/utils/format';
import { reasonFor } from '@/services/task-error';
import { taskTitle } from '@/services/task-title';

const props = defineProps<{ task: Task; selectMode?: boolean; selected?: boolean }>();

// A readable title (the download's folder name) plus the season/episode for a
// series, in place of Download Station's raw file name.
const heading = computed(() => taskTitle(props.task));

// For downloads sent from Discover, a capitalized media type (Movie / Series) to
// show alongside the rating and year.
const mediaLabel = computed(() =>
  props.task.mediaType ? props.task.mediaType[0].toUpperCase() + props.task.mediaType.slice(1) : '',
);
const emit = defineEmits<{
  (e: 'pause', id: string): void;
  (e: 'resume', id: string): void;
  (e: 'delete', id: string): void;
  (e: 'toggle', id: string): void;
  (e: 'open', id: string): void;
}>();

// After a swipe action fires, slide the row back to its closed state so it
// doesn't linger open over the (now-changed) task.
const sliding = ref<InstanceType<typeof IonItemSliding> | null>(null);
function closeSlide(): void {
  void (sliding.value?.$el as HTMLIonItemSlidingElement | undefined)?.close();
}
function onPause(): void {
  emit('pause', props.task.id);
  closeSlide();
}
function onResume(): void {
  emit('resume', props.task.id);
  closeSlide();
}

// In selection mode, tapping the row toggles selection (swipe actions are off);
// otherwise it opens the task detail view (spec 0002 US3).
function onRowClick(): void {
  if (props.selectMode) emit('toggle', props.task.id);
  else emit('open', props.task.id);
}

// Pause only makes sense for active work; resume only for paused.
const canPause = computed(() =>
  ['downloading', 'waiting', 'filehosting_waiting'].includes(props.task.status),
);
const canResume = computed(() => props.task.status === 'paused');

const active = computed(() => props.task.status === 'downloading');
const statusColorVar = computed(() => {
  switch (props.task.status) {
    case 'downloading':
      return 'var(--app-status-downloading)';
    case 'finished':
      return 'var(--app-status-finished)';
    case 'seeding':
      return 'var(--app-status-seeding)';
    case 'paused':
      return 'var(--app-status-paused)';
    case 'error':
      return 'var(--app-status-error)';
    default:
      return 'var(--app-status-waiting)';
  }
});
const eta = computed(() =>
  formatEta(props.task.size - props.task.downloaded, props.task.downloadSpeed),
);
// Errored tasks show *why* they failed (spec 0002) instead of a bare "error".
const errorReason = computed(() =>
  props.task.status === 'error' ? reasonFor(props.task.errorDetail ?? '') : '',
);
</script>

<template>
  <ion-item-sliding ref="sliding" :disabled="selectMode">
    <ion-item :detail="false" button data-testid="task-item" @click="onRowClick">
      <ion-checkbox
        v-if="selectMode"
        slot="start"
        :checked="selected"
        aria-label="Select task"
        data-testid="task-select"
      />
      <ion-label>
        <h2 class="name">
          {{ heading.title }}<span v-if="heading.episode" class="ep">{{ heading.episode }}</span>
        </h2>
        <div v-if="task.mediaType" class="media" data-testid="task-media">
          <span class="type">{{ mediaLabel }}</span>
          <span v-if="task.year">{{ task.year }}</span>
          <span v-if="task.imdbScore">★ {{ task.imdbScore.toFixed(1) }}</span>
        </div>
        <div class="meta">
          <span class="status" :style="{ color: statusColorVar }" data-testid="task-status">
            {{ task.status }}
          </span>
          <span v-if="errorReason" class="reason" data-testid="task-error-reason">{{ errorReason }}</span>
          <span v-else>{{ formatPercent(task.downloaded, task.size) }} of {{ formatBytes(task.size) }}</span>
          <span v-if="active">↓ {{ formatSpeed(task.downloadSpeed) }}</span>
          <span v-if="active && task.uploadSpeed > 0">↑ {{ formatSpeed(task.uploadSpeed) }}</span>
          <span v-if="active">{{ eta }}</span>
          <span v-if="task.addedBy" class="added-by" data-testid="task-added-by">added by {{ task.addedBy }}</span>
        </div>
        <ion-progress-bar
          v-if="task.status !== 'finished'"
          :value="progressOf(task.downloaded, task.size)"
          :style="{ '--progress-background': statusColorVar }"
        />
      </ion-label>
    </ion-item>
    <ion-item-options side="end">
      <ion-item-option v-if="canPause" data-testid="task-pause" @click="onPause">
        <ion-icon slot="icon-only" :icon="pauseOutline" />
      </ion-item-option>
      <ion-item-option v-if="canResume" color="success" data-testid="task-resume" @click="onResume">
        <ion-icon slot="icon-only" :icon="playOutline" />
      </ion-item-option>
      <ion-item-option color="danger" data-testid="task-delete" @click="emit('delete', task.id)">
        <ion-icon slot="icon-only" :icon="trashOutline" />
      </ion-item-option>
    </ion-item-options>
  </ion-item-sliding>
</template>

<style scoped>
.name {
  font-size: 0.95rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ep {
  margin-left: 6px;
  padding: 1px 6px;
  border-radius: 6px;
  font-size: 0.72rem;
  font-weight: 600;
  color: var(--ion-color-primary);
  background: rgba(var(--ion-color-primary-rgb, 16, 185, 129), 0.14);
}
.media {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.75rem;
  color: var(--app-text-dim);
  margin-top: 0.15rem;
}
.media .type {
  padding: 1px 6px;
  border-radius: 6px;
  font-weight: 600;
  color: var(--ion-color-primary);
  background: rgba(var(--ion-color-primary-rgb, 16, 185, 129), 0.14);
}
.meta {
  display: flex;
  gap: 0.75rem;
  font-size: 0.78rem;
  color: var(--app-text-dim);
  margin: 0.2rem 0 0.35rem;
}
.status {
  text-transform: capitalize;
  font-weight: 600;
}
.reason {
  color: var(--app-status-error);
}
.added-by {
  font-style: italic;
  opacity: 0.85;
}
ion-progress-bar {
  height: 3px;
  border-radius: 2px;
}
</style>
