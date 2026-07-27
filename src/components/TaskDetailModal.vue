<script setup lang="ts">
/**
 * Task detail sheet (spec 0002 US3). Stock Ionic modal bound to the live task
 * BY ID from the reactive task collection, so its fields update in place while
 * open. If the task disappears from the NAS (deleted/cleared elsewhere), the
 * `task` prop goes null and the sheet shows a gone state instead of stale data.
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
  IonNote,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { computed } from 'vue';
import type { Task } from '@/types/task';
import { formatBytes, formatDate, formatSpeed, formatPercent } from '@/utils/format';
import { reasonFor } from '@/services/task-error';

const props = defineProps<{ isOpen: boolean; task: Task | null }>();
defineEmits<{ (e: 'dismiss'): void }>();

const reason = computed(() =>
  props.task?.status === 'error' ? reasonFor(props.task.errorDetail ?? '') : '',
);
</script>

<template>
  <ion-modal :is-open="isOpen" @did-dismiss="$emit('dismiss')">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>Task details</ion-title>
        <ion-buttons slot="end">
          <ion-button data-testid="detail-close" @click="$emit('dismiss')">Close</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content>
      <div v-if="!task" class="gone" data-testid="detail-gone">
        <p>This task is no longer available.</p>
      </div>
      <ion-list v-else data-testid="task-detail">
        <ion-item>
          <ion-label class="ion-text-wrap">
            <p>Name</p>
            <h2 data-testid="detail-name">{{ task.name }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <p>Status</p>
            <h2 data-testid="detail-status">{{ task.status }}</h2>
          </ion-label>
        </ion-item>
        <ion-item v-if="reason">
          <ion-label class="ion-text-wrap">
            <p>Reason</p>
            <h2 data-testid="detail-reason">{{ reason }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label class="ion-text-wrap">
            <p>Destination</p>
            <h2 data-testid="detail-destination">{{ task.destination || '—' }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <p>Created</p>
            <h2>{{ formatDate(task.createdAt) }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <p>Progress</p>
            <h2 data-testid="detail-progress">
              {{ formatPercent(task.downloaded, task.size) }} —
              {{ formatBytes(task.downloaded) }} of {{ formatBytes(task.size) }}
            </h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <p>Uploaded</p>
            <h2>{{ formatBytes(task.uploaded) }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <p>Speeds</p>
            <h2>↓ {{ formatSpeed(task.downloadSpeed) }} · ↑ {{ formatSpeed(task.uploadSpeed) }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <p>Peers / seeders</p>
            <h2>{{ task.peers }} / {{ task.seeders }}</h2>
          </ion-label>
          <ion-note slot="end">{{ task.type }}</ion-note>
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
</style>
