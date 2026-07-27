<script setup lang="ts">
import {
  actionSheetController,
  IonButton,
  IonButtons,
  IonContent,
  IonFab,
  IonFabButton,
  IonHeader,
  IonIcon,
  IonList,
  IonPage,
  IonRefresher,
  IonRefresherContent,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { addOutline, optionsOutline } from 'ionicons/icons';
import { computed, ref } from 'vue';
import { useTasks } from '@/composables/useTasks';
import { useTaskFilter } from '@/composables/useTaskFilter';
import { api } from '@/services/api';
import { applyTaskFilter, type TaskFilterState } from '@/services/task-sort';
import { formatSpeed } from '@/utils/format';
import TaskItem from '@/components/TaskItem.vue';
import TaskFilterSheet from '@/components/TaskFilterSheet.vue';
import NewTaskModal from '@/components/NewTaskModal.vue';
import type { RefresherCustomEvent } from '@ionic/vue';

const { tasks, stats, loaded, refresh } = useTasks();
const { filter, apply } = useTaskFilter();

const filterOpen = ref(false);
const newTaskOpen = ref(false);

const visible = computed(() => applyTaskFilter(tasks.value, filter.value));

async function onPull(ev: RefresherCustomEvent): Promise<void> {
  await refresh();
  await ev.target.complete();
}

async function onApplyFilter(next: TaskFilterState): Promise<void> {
  await apply(next);
  filterOpen.value = false;
}

async function onCreated(): Promise<void> {
  newTaskOpen.value = false;
  await refresh();
}

async function onPause(id: string): Promise<void> {
  await api.pauseTasks([id]);
  await refresh();
}

async function onResume(id: string): Promise<void> {
  await api.resumeTasks([id]);
  await refresh();
}

// Delete needs a confirmation (US4): an action sheet, never a silent remove.
async function onDelete(id: string): Promise<void> {
  const sheet = await actionSheetController.create({
    header: 'Delete this download task?',
    subHeader: 'Completed files stay on the NAS.',
    buttons: [
      { text: 'Delete task', role: 'destructive', data: 'delete' },
      { text: 'Cancel', role: 'cancel' },
    ],
  });
  await sheet.present();
  const { data } = await sheet.onDidDismiss();
  if (data === 'delete') {
    await api.deleteTasks([id]);
    await refresh();
  }
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
        <ion-buttons slot="end">
          <ion-button data-testid="filter-open" @click="filterOpen = true">
            <ion-icon slot="icon-only" :icon="optionsOutline" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content>
      <ion-refresher slot="fixed" @ionRefresh="onPull">
        <ion-refresher-content />
      </ion-refresher>

      <div v-if="!loaded" class="center"><ion-spinner name="crescent" /></div>
      <div v-else-if="visible.length === 0" class="center empty" data-testid="tasks-empty">
        <p>{{ tasks.length === 0 ? 'No download tasks.' : 'No tasks match the filters.' }}</p>
      </div>
      <ion-list v-else data-testid="task-list">
        <TaskItem
          v-for="t in visible"
          :key="t.id"
          :task="t"
          @pause="onPause"
          @resume="onResume"
          @delete="onDelete"
        />
      </ion-list>

      <ion-fab slot="fixed" vertical="bottom" horizontal="end">
        <ion-fab-button data-testid="newtask-open" @click="newTaskOpen = true">
          <ion-icon :icon="addOutline" />
        </ion-fab-button>
      </ion-fab>
    </ion-content>

    <TaskFilterSheet
      :is-open="filterOpen"
      :filter="filter"
      @apply="onApplyFilter"
      @dismiss="filterOpen = false"
    />
    <NewTaskModal :is-open="newTaskOpen" @created="onCreated" @dismiss="newTaskOpen = false" />
  </ion-page>
</template>

<style scoped>
.speeds {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  font-size: 0.7rem;
  color: var(--app-text-dim);
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
