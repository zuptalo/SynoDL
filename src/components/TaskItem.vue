<script setup lang="ts">
import { IonItem, IonLabel, IonProgressBar } from '@ionic/vue';
import { computed } from 'vue';
import type { Task } from '@/types/task';
import { formatBytes, formatEta, formatPercent, formatSpeed, progressOf } from '@/utils/format';

const props = defineProps<{ task: Task }>();

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
  <ion-item :detail="false" data-testid="task-item">
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
