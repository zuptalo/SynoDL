<script setup lang="ts">
import {
  IonContent,
  IonHeader,
  IonList,
  IonPage,
  IonRefresher,
  IonRefresherContent,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { useTasks } from '@/composables/useTasks';
import { formatSpeed } from '@/utils/format';
import TaskItem from '@/components/TaskItem.vue';
import type { RefresherCustomEvent } from '@ionic/vue';

const { tasks, stats, loaded, refresh } = useTasks();

async function onPull(ev: RefresherCustomEvent): Promise<void> {
  await refresh();
  await ev.target.complete();
}
</script>

<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-title>Tasks</ion-title>
        <div slot="end" class="speeds" data-testid="global-speeds">
          <span>↓ {{ formatSpeed(stats.downloadSpeed) }}</span>
          <span>↑ {{ formatSpeed(stats.uploadSpeed) }}</span>
        </div>
      </ion-toolbar>
    </ion-header>
    <ion-content>
      <ion-refresher slot="fixed" @ionRefresh="onPull">
        <ion-refresher-content />
      </ion-refresher>

      <div v-if="!loaded" class="center"><ion-spinner name="crescent" /></div>
      <div v-else-if="tasks.length === 0" class="center empty" data-testid="tasks-empty">
        <p>No download tasks.</p>
      </div>
      <ion-list v-else data-testid="task-list">
        <TaskItem v-for="t in tasks" :key="t.id" :task="t" />
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<style scoped>
.speeds {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  font-size: 0.7rem;
  color: var(--app-text-dim);
  padding-inline-end: 1rem;
  line-height: 1.25;
}
.center {
  display: flex;
  justify-content: center;
  padding-top: 30vh;
}
.empty p {
  color: var(--app-text-dim);
}
</style>
