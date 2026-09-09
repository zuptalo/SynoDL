<script setup lang="ts">
import {
  actionSheetController,
  IonButton,
  IonButtons,
  IonContent,
  IonFab,
  IonFabList,
  IonFabButton,
  IonHeader,
  IonSearchbar,
  IonIcon,
  IonLabel,
  IonList,
  IonListHeader,
  IonNote,
  IonPage,
  IonRefresher,
  IonRefresherContent,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import {
  addOutline,
  checkmarkOutline,
  cloudUploadOutline,
  ellipsisHorizontal,
  linkOutline,
  musicalNotesOutline,
  optionsOutline,
} from 'ionicons/icons';
import { computed, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useTasks } from '@/composables/useTasks';
import { useTaskFilter } from '@/composables/useTaskFilter';
import { api, type YtdlDownload } from '@/services/api';
import { ALL_STATUSES, applyTaskFilter, type TaskFilterState } from '@/services/task-sort';
import { formatSpeed } from '@/utils/format';
import type { Task } from '@/types/task';
import TaskItem from '@/components/TaskItem.vue';
import TaskFilterSheet from '@/components/TaskFilterSheet.vue';
import NewTaskModal from '@/components/NewTaskModal.vue';
import UploadModal from '@/components/UploadModal.vue';
import { useUploads } from '@/composables/useUploads';
import UploadItem from '@/components/UploadItem.vue';
import { useYtdl } from '@/composables/useYtdl';
import YtdlItem from '@/components/YtdlItem.vue';
import YtdlDetailModal from '@/components/YtdlDetailModal.vue';
import YtdlGroupModal from '@/components/YtdlGroupModal.vue';
import YoutubeDownloadModal from '@/components/YoutubeDownloadModal.vue';
import TaskDetailModal from '@/components/TaskDetailModal.vue';
import type { RefresherCustomEvent } from '@ionic/vue';

const { tasks, stats, loaded, refresh } = useTasks();
const { filter, apply } = useTaskFilter();

const filterOpen = ref(false);
const newTaskOpen = ref(false);
const uploadOpen = ref(false);
// Its own sheet rather than a mode inside the new-task one (spec 1033): that
// sheet leads with a destination picker which means nothing for a library
// download, and led with it while still looking interactive.
const ytdlOpen = ref(false);
// Bound BY ID rather than by object, matching how the task detail sheet works:
// the sheet then follows the live download, so a progress bar keeps moving while
// it is open and a download that is dismissed elsewhere shows a gone state
// instead of stale data.
const ytdlDetailId = ref<string | null>(null);
// A group opens its CONTENTS rather than a detail sheet: what someone wants
// from a channel row is the list of what it is doing (FR-019a).
const ytdlGroupId = ref<string | null>(null);

// Uploads report HERE as well as in the sheet, so dismissing the sheet is a UI
// choice rather than losing sight of a transfer that is still running. A job
// only leaves this list when it is dismissed, so a failure cannot go unnoticed.
const { jobs: uploads, retry: retryUpload, cancel: cancelUpload, dismiss: dismissUpload } =
  useUploads();

// YouTube downloads (spec 0012) report here alongside NAS downloads. They are a
// SEPARATE list rather than rows merged into `tasks`, and deliberately so: the
// two systems have different states and different capabilities, and keeping
// them apart in code is what makes it impossible for a NAS-only bulk action to
// reach one (FR-028). They still read as one list to the user.
const {
  downloads: ytdlDownloads,
  available: ytdlAvailable,
  dismiss: dismissYtdl,
  retry: retryYtdl,
} = useYtdl();

// A status filter names NAS statuses ("downloading", "seeding", …) that a
// YouTube download cannot have, so a NARROWED one hides them rather than
// pretending they match. Narrowed is the operative word: the default filter has
// every status enabled, which means "nothing is filtered out" — treating that
// as an active filter would hide these rows permanently. A text search still
// applies either way.
const statusFilterNarrowed = computed(
  () => filter.value.statuses.length > 0 && filter.value.statuses.length < ALL_STATUSES.length,
);
const visibleYtdl = computed(() => {
  if (!ytdlAvailable.value) return [];
  if (statusFilterNarrowed.value) return [];
  const term = (filter.value.term ?? '').trim().toLowerCase();
  if (!term) return ytdlDownloads.value;
  return ytdlDownloads.value.filter((d) => d.url.toLowerCase().includes(term));
});

async function onDismissYtdl(requestId: string): Promise<void> {
  try {
    await dismissYtdl(requestId);
  } catch {
    // The next poll is the source of truth; a failed dismiss simply reappears.
  }
}
async function onRetryYtdl(requestId: string): Promise<void> {
  try {
    await retryYtdl(requestId);
  } catch {
    // Same reasoning: the row reflects whatever the server says next. A retry
    // the server refused (it had already been retried, say) simply stays failed.
  }
}
// The warning only belongs on screen while something is actually in flight.
const uploadRunning = computed(() =>
  uploads.value.some((j) => j.state === 'sending' || j.state === 'waiting'),
);

const visible = computed(() => applyTaskFilter(tasks.value, filter.value));

// ---- detail view (spec 0002 US3) ------------------------------------------
// Track the open task by id so the sheet re-reads the live task on every
// snapshot; it goes null (gone state) if the task leaves the list.
const detailId = ref<string | null>(null);
const detailTask = computed<Task | null>(
  () => tasks.value.find((t) => t.id === detailId.value) ?? null,
);
function openDetail(id: string): void {
  detailId.value = id;
}

// Looked up from the live collection each render, for the same reason
// detailTask is: the sheet must follow the download, not a snapshot of it.
const ytdlDetail = computed<YtdlDownload | null>(
  () => ytdlDownloads.value.find((d) => d.requestId === ytdlDetailId.value) ?? null,
);
const ytdlGroup = computed<YtdlDownload | null>(
  () => ytdlDownloads.value.find((d) => d.requestId === ytdlGroupId.value) ?? null,
);
function onOpenYtdl(requestId: string): void {
  const row = ytdlDownloads.value.find((d) => d.requestId === requestId);
  if (row?.kind === 'group') ytdlGroupId.value = requestId;
  else ytdlDetailId.value = requestId;
}

// Deep link from a tapped download notification: /tabs/tasks?task=<id> opens
// that task's detail once the list has loaded (so the sheet resolves the live
// task), then clears the query so it doesn't reopen on back/refresh.
const route = useRoute();
const router = useRouter();
watch(
  [() => route.query.task, loaded],
  ([id, isLoaded]) => {
    if (isLoaded && typeof id === 'string' && id) {
      openDetail(id);
      void router.replace({ query: {} });
    }
  },
  { immediate: true },
);

// ---- selection mode -------------------------------------------------------
const selectMode = ref(false);
const selected = ref<Set<string>>(new Set());
const selectedCount = computed(() => selected.value.size);

function enterSelect(): void {
  selectMode.value = true;
  selected.value = new Set();
}
function cancelSelect(): void {
  selectMode.value = false;
  selected.value = new Set();
}
function toggleSelect(id: string): void {
  const next = new Set(selected.value);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  selected.value = next;
}

// Keep the selection valid across live refreshes: drop tasks that vanished.
watch(tasks, (list) => {
  if (!selectMode.value) return;
  const ids = new Set(list.map((t) => t.id));
  const kept = new Set([...selected.value].filter((id) => ids.has(id)));
  if (kept.size !== selected.value.size) selected.value = kept;
});

// ---- eligibility + bulk runners -------------------------------------------
const PAUSABLE = ['downloading', 'waiting', 'filehosting_waiting'];
const pausable = (pool: Task[]) => pool.filter((t) => PAUSABLE.includes(t.status)).map((t) => t.id);
const resumable = (pool: Task[]) => pool.filter((t) => t.status === 'paused').map((t) => t.id);
const finished = (pool: Task[]) => pool.filter((t) => t.status === 'finished').map((t) => t.id);
const plural = (n: number) => (n === 1 ? 'task' : 'tasks');

async function runPause(ids: string[]): Promise<void> {
  if (ids.length) {
    await api.pauseTasks(ids);
    await refresh();
  }
}
async function runResume(ids: string[]): Promise<void> {
  if (ids.length) {
    await api.resumeTasks(ids);
    await refresh();
  }
}
// Every destructive bulk action confirms first, naming the count.
async function confirmDelete(ids: string[], subHeader: string): Promise<boolean> {
  if (!ids.length) return false;
  const sheet = await actionSheetController.create({
    header: `Delete ${ids.length} ${plural(ids.length)}?`,
    subHeader,
    buttons: [
      { text: `Delete ${ids.length}`, role: 'destructive', data: 'ok' },
      { text: 'Cancel', role: 'cancel' },
    ],
  });
  await sheet.present();
  const { data } = await sheet.onDidDismiss();
  if (data !== 'ok') return false;
  await api.deleteTasks(ids);
  await refresh();
  return true;
}
async function clearFinished(pool: Task[]): Promise<void> {
  const ids = finished(pool);
  await confirmDelete(ids, 'Removes the finished entries; files stay on the NAS.');
}

// ---- menus ----------------------------------------------------------------
async function openOverflow(): Promise<void> {
  const all = tasks.value;
  const sheet = await actionSheetController.create({
    header: `${all.length} ${plural(all.length)}`,
    buttons: [
      { text: 'Select tasks', data: 'select' },
      { text: 'Pause all', data: 'pause' },
      { text: 'Resume all', data: 'resume' },
      { text: 'Delete all', role: 'destructive', data: 'delete' },
      { text: `Clear finished (${finished(all).length})`, role: 'destructive', data: 'clear' },
      { text: 'Cancel', role: 'cancel' },
    ],
  });
  await sheet.present();
  const { data } = await sheet.onDidDismiss();
  switch (data) {
    case 'select':
      enterSelect();
      break;
    case 'clear':
      await clearFinished(all);
      break;
    case 'pause':
      await runPause(pausable(all));
      break;
    case 'resume':
      await runResume(resumable(all));
      break;
    case 'delete':
      await confirmDelete(
        all.map((t) => t.id),
        'Removes every task; files stay on the NAS.',
      );
      break;
  }
}

async function openSelectionActions(): Promise<void> {
  if (selectedCount.value === 0) return;
  const pool = tasks.value.filter((t) => selected.value.has(t.id));
  const sheet = await actionSheetController.create({
    header: `${pool.length} ${plural(pool.length)} selected`,
    buttons: [
      { text: `Clear finished (${finished(pool).length})`, role: 'destructive', data: 'clear' },
      { text: 'Pause', data: 'pause' },
      { text: 'Resume', data: 'resume' },
      { text: 'Delete', role: 'destructive', data: 'delete' },
      { text: 'Cancel', role: 'cancel' },
    ],
  });
  await sheet.present();
  const { data } = await sheet.onDidDismiss();
  switch (data) {
    case 'clear':
      await clearFinished(pool);
      cancelSelect();
      break;
    case 'pause':
      await runPause(pausable(pool));
      cancelSelect();
      break;
    case 'resume':
      await runResume(resumable(pool));
      cancelSelect();
      break;
    case 'delete':
      if (await confirmDelete(pool.map((t) => t.id), 'Completed files stay on the NAS.')) cancelSelect();
      break;
  }
}

// ---- existing per-row + list plumbing -------------------------------------
async function onPull(ev: RefresherCustomEvent): Promise<void> {
  await refresh();
  await ev.target.complete();
}
async function onApplyFilter(next: TaskFilterState): Promise<void> {
  await apply(next);
  filterOpen.value = false;
}
function onSearch(ev: CustomEvent): void {
  const term = String((ev.target as HTMLInputElement | null)?.value ?? '');
  void apply({ ...filter.value, term });
}
async function onCreated(): Promise<void> {
  // Both sheets hand off the same way: the work is queued, so close and let the
  // list report it. An upload that stayed open would duplicate the rows the
  // Tasks list already shows, and leave the user deciding which one to watch.
  newTaskOpen.value = false;
  uploadOpen.value = false;
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
async function onDelete(id: string): Promise<void> {
  await confirmDelete([id], 'Completed files stay on the NAS.');
}
</script>

<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-button v-if="selectMode" data-testid="select-cancel" @click="cancelSelect">Cancel</ion-button>
          <ion-button v-else data-testid="overflow-open" @click="openOverflow">
            <ion-icon slot="icon-only" :icon="ellipsisHorizontal" />
          </ion-button>
        </ion-buttons>
        <ion-title>{{ selectMode ? `${selectedCount} selected` : 'Tasks' }}</ion-title>
        <div v-if="!selectMode" slot="end" class="speeds" data-testid="global-speeds">
          <span>↓ {{ formatSpeed(stats.downloadSpeed) }}</span>
          <span>↑ {{ formatSpeed(stats.uploadSpeed) }}</span>
        </div>
        <ion-buttons v-if="!selectMode" slot="end">
          <ion-button data-testid="filter-open" @click="filterOpen = true">
            <ion-icon slot="icon-only" :icon="optionsOutline" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
      <!-- Always-visible search of the download list, by name (spec 1013). -->
      <ion-toolbar v-if="!selectMode">
        <ion-searchbar
          :value="filter.term"
          placeholder="Search downloads"
          :debounce="150"
          data-testid="tasks-search"
          @ionInput="onSearch"
        />
      </ion-toolbar>
    </ion-header>
    <ion-content :fullscreen="true">
      <ion-refresher slot="fixed" @ionRefresh="onPull">
        <ion-refresher-content />
      </ion-refresher>

      <!-- Uploads sit above the downloads and read the same way: they are
           transfers in progress, not notices. -->
      <ion-list v-if="uploads.length" data-testid="upload-list">
        <ion-list-header><ion-label>Uploads</ion-label></ion-list-header>
        <UploadItem
          v-for="job in uploads"
          :key="job.id"
          :job="job"
          @stop="cancelUpload"
          @retry="retryUpload"
          @clear="dismissUpload"
        />
        <ion-note v-if="uploadRunning" class="upload-hint" color="medium">
          Keep the app open while an upload is running.
        </ion-note>
      </ion-list>

      <!-- YouTube downloads sit with the NAS ones and read the same way: they
           are downloads in progress. They are a separate list because they are
           a separate system — no pause, no resume, no progress. -->
      <ion-list v-if="visibleYtdl.length" data-testid="ytdl-list">
        <ion-list-header><ion-label>From YouTube</ion-label></ion-list-header>
        <YtdlItem
          v-for="d in visibleYtdl"
          :key="d.requestId"
          :download="d"
          @dismiss="onDismissYtdl"
          @open="onOpenYtdl"
          @retry="onRetryYtdl"
        />
      </ion-list>

      <div v-if="!loaded" class="center"><ion-spinner name="crescent" /></div>
      <div v-else-if="visible.length === 0" class="center empty" data-testid="tasks-empty">
        <p>{{ tasks.length === 0 ? 'No download tasks.' : 'No tasks match the filters.' }}</p>
      </div>
      <ion-list v-else data-testid="task-list">
        <TaskItem
          v-for="t in visible"
          :key="t.id"
          :task="t"
          :select-mode="selectMode"
          :selected="selected.has(t.id)"
          @pause="onPause"
          @resume="onResume"
          @delete="onDelete"
          @toggle="toggleSelect"
          @open="openDetail"
        />
      </ion-list>

      <ion-fab slot="fixed" vertical="bottom" horizontal="end">
        <!-- Selection mode swaps the create button for a confirm checkmark,
             disabled until at least one task is selected. -->
        <ion-fab-button
          v-if="selectMode"
          class="app-fab"
          :disabled="selectedCount === 0"
          data-testid="selection-confirm"
          @click="openSelectionActions"
        >
          <ion-icon :icon="checkmarkOutline" />
        </ion-fab-button>
        <!-- Two ways to put something in the library now: fetch it by URL, or
             send a file from this device (spec 1022). An ion-fab-list is the
             stock way to offer both without a second floating button. -->
        <template v-else>
          <ion-fab-button class="app-fab" data-testid="newtask-fab">
            <ion-icon :icon="addOutline" />
          </ion-fab-button>
          <ion-fab-list side="top">
            <ion-fab-button
              class="app-fab"
              title="Add by URL"
              data-testid="newtask-open"
              @click="newTaskOpen = true"
            >
              <ion-icon :icon="linkOutline" />
            </ion-fab-button>
            <ion-fab-button
              class="app-fab"
              title="Upload a file"
              data-testid="upload-open"
              @click="uploadOpen = true"
            >
              <ion-icon :icon="cloudUploadOutline" />
            </ion-fab-button>
            <!-- Offered only where the server can actually run a worker, so the
                 button never opens a sheet that cannot work (FR-002). -->
            <ion-fab-button
              v-if="ytdlAvailable"
              class="app-fab"
              title="Save from YouTube"
              data-testid="ytdl-open"
              @click="ytdlOpen = true"
            >
              <ion-icon :icon="musicalNotesOutline" />
            </ion-fab-button>
          </ion-fab-list>
        </template>
      </ion-fab>
    </ion-content>

    <TaskFilterSheet
      :is-open="filterOpen"
      :filter="filter"
      @apply="onApplyFilter"
      @dismiss="filterOpen = false"
    />
    <NewTaskModal :is-open="newTaskOpen" @created="onCreated" @dismiss="newTaskOpen = false" />
    <UploadModal :is-open="uploadOpen" @uploaded="onCreated" @dismiss="uploadOpen = false" />
    <YoutubeDownloadModal
      :is-open="ytdlOpen"
      @created="ytdlOpen = false"
      @dismiss="ytdlOpen = false"
    />
    <TaskDetailModal
      :is-open="detailId !== null"
      :task="detailTask"
      @dismiss="detailId = null"
    />
    <YtdlDetailModal
      :is-open="ytdlDetailId !== null"
      :request-id="ytdlDetailId"
      :download="ytdlDetail"
      @dismiss="ytdlDetailId = null"
      @retry="onRetryYtdl"
    />
    <YtdlGroupModal
      :is-open="ytdlGroupId !== null"
      :group="ytdlGroup"
      @dismiss="ytdlGroupId = null"
      @retry="onRetryYtdl"
      @open="ytdlDetailId = $event"
    />
  </ion-page>
</template>

<style scoped>
.bad {
  color: var(--ion-color-danger);
}
.upload-hint {
  display: block;
  padding: 4px 16px 8px;
  font-size: 0.75rem;
}
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
