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
import { computed } from 'vue';
import type { Task } from '@/types/task';
import { formatBytes, formatEta, formatPercent, formatSpeed, progressOf } from '@/utils/format';

const props = defineProps<{ task: Task; selectMode?: boolean; selected?: boolean }>();
const emit = defineEmits<{
  (e: 'pause', id: string): void;
  (e: 'resume', id: string): void;
  (e: 'delete', id: string): void;
  (e: 'toggle', id: string): void;
}>();

// In selection mode, tapping the row toggles selection (swipe actions are off).
function onRowClick(): void {
  if (props.selectMode) emit('toggle', props.task.id);
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
</script>

<template>
  <ion-item-sliding :disabled="selectMode">
    <ion-item :detail="false" :button="selectMode" data-testid="task-item" @click="onRowClick">
      <ion-checkbox
        v-if="selectMode"
        slot="start"
        :checked="selected"
        aria-label="Select task"
        data-testid="task-select"
      />
      <ion-label>
        <h2 class="name">{{ task.name }}</h2>
        <div class="meta">
          <span class="status" :style="{ color: statusColorVar }" data-testid="task-status">
            {{ task.status }}
          </span>
          <span>{{ formatPercent(task.downloaded, task.size) }} of {{ formatBytes(task.size) }}</span>
          <span v-if="active">↓ {{ formatSpeed(task.downloadSpeed) }}</span>
          <span v-if="active && task.uploadSpeed > 0">↑ {{ formatSpeed(task.uploadSpeed) }}</span>
          <span v-if="active">{{ eta }}</span>
        </div>
        <ion-progress-bar
          v-if="task.status !== 'finished'"
          :value="progressOf(task.downloaded, task.size)"
          :style="{ '--progress-background': statusColorVar }"
        />
      </ion-label>
    </ion-item>
    <ion-item-options side="end">
      <ion-item-option v-if="canPause" data-testid="task-pause" @click="emit('pause', task.id)">
        <ion-icon slot="icon-only" :icon="pauseOutline" />
      </ion-item-option>
      <ion-item-option v-if="canResume" color="success" data-testid="task-resume" @click="emit('resume', task.id)">
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
ion-progress-bar {
  height: 3px;
  border-radius: 2px;
}
</style>
