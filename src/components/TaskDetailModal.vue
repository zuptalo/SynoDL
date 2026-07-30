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
  IonFooter,
  IonHeader,
  IonIcon,
  IonItem,
  IonLabel,
  IonList,
  IonModal,
  IonNote,
  IonProgressBar,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { copyOutline, filmOutline, refreshOutline } from 'ionicons/icons';
import { computed } from 'vue';
import { useRouter } from 'vue-router';
import { api, type CatalogTitle } from '@/services/api';
import { appToast } from '@/services/toast';
import type { Task } from '@/types/task';
import { formatBytes, formatDate, formatSpeed, formatPercent } from '@/utils/format';
import { reasonFor } from '@/services/task-error';
import { taskTitle } from '@/services/task-title';
import { useSourceCatalog } from '@/composables/useSourceCatalog';

const props = defineProps<{ isOpen: boolean; task: Task | null }>();
const emit = defineEmits<{ (e: 'dismiss'): void }>();
const router = useRouter();
const { requestOpen } = useSourceCatalog();

const reason = computed(() =>
  props.task?.status === 'error' ? reasonFor(props.task.errorDetail ?? '') : '',
);

// Capitalized media type (Movie / Series) for a Discover-sent download.
const mediaLabel = computed(() => {
  const t = props.task?.mediaType;
  return t ? t[0].toUpperCase() + t.slice(1) : '';
});

// Torrent-only stats (upload total, upload speed, peers/seeders) are meaningless
// for HTTP/FTP/NZB/eMule downloads, so they're hidden for non-bt tasks (spec 1016).
const isTorrent = computed(() => props.task?.type === 'bt');

// "Open in Discover" is offered for downloads that carry catalog metadata.
const canOpenInDiscover = computed(() => !!props.task?.mediaType);

// Hand the title to the Browser tab and switch to it: the exact title when we
// stored its catalog id, otherwise a search of the (year-stripped) title.
async function openInDiscover(): Promise<void> {
  const t = props.task;
  if (!t) return;
  const clean = taskTitle(t).title;
  if (t.catalogId) {
    const ct: CatalogTitle = {
      id: t.catalogId,
      type: t.mediaType ?? '',
      title: clean,
      posterUrl: t.posterUrl ?? '',
      imdbId: '',
      imdbScore: t.imdbScore ?? 0,
      providerScore: 0,
      plot: '',
      genres: [],
      comingSoon: false,
      freeDownload: false,
    };
    requestOpen(ct);
  } else {
    requestOpen(null, clean);
  }
  emit('dismiss');
  await router.push('/tabs/browser');
}

// Re-download makes sense for a task that has finished or failed and still knows
// its source link (spec 1007).
const canRedownload = computed(
  () => !!props.task?.uri && ['finished', 'error'].includes(props.task?.status ?? ''),
);

async function copyLink(): Promise<void> {
  if (!props.task?.uri) return;
  try {
    await navigator.clipboard.writeText(props.task.uri);
    await appToast({ message: 'Link copied.', duration: 1800 });
  } catch {
    await appToast({ message: 'Could not copy the link.', color: 'danger', duration: 2200 });
  }
}

async function redownload(): Promise<void> {
  const t = props.task;
  if (!t?.uri) return;
  const { uri, id } = t;
  const destination = t.destination || undefined;
  try {
    // Replace the existing entry in place: remove the finished/failed task first
    // so Download Station adds a fresh task instead of a numbered duplicate
    // ("name (1)") beside the old one. Deleting a task keeps its files on the NAS.
    await api.deleteTasks([id]);
    await api.createTaskURIs([uri], { destination });
    await appToast({ message: 'Re-download started.', duration: 1800 });
    emit('dismiss'); // the old task is gone; close rather than show the "gone" state
  } catch {
    await appToast({ message: 'Could not restart the download.', color: 'danger', duration: 2200 });
  }
}
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
        <!-- Attribution is sent only to admins, so this row simply doesn't
             appear for a regular user. -->
        <ion-item v-if="task.addedBy">
          <ion-label class="ion-text-wrap">
            <p>Added by</p>
            <h2 data-testid="detail-added-by">{{ task.addedBy }}</h2>
          </ion-label>
        </ion-item>
        <!-- Catalog metadata for a download sent from Discover. -->
        <ion-item v-if="task.mediaType">
          <ion-label class="ion-text-wrap">
            <p>Catalog</p>
            <h2 data-testid="detail-media">
              {{ mediaLabel }}<template v-if="task.year"> · {{ task.year }}</template
              ><template v-if="task.imdbScore"> · ★ {{ task.imdbScore.toFixed(1) }}</template>
            </h2>
          </ion-label>
        </ion-item>
        <ion-item v-if="task.uri">
          <ion-label class="ion-text-wrap">
            <p>Link</p>
            <h2 data-testid="detail-uri">{{ task.uri }}</h2>
          </ion-label>
          <ion-button slot="end" fill="clear" data-testid="detail-copy" @click="copyLink">
            <ion-icon slot="icon-only" :icon="copyOutline" />
          </ion-button>
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
            <ion-progress-bar
              class="detail-progress-bar"
              :color="task.status === 'error' ? 'danger' : 'primary'"
              :value="task.size > 0 ? Math.min(1, task.downloaded / task.size) : 0"
            />
          </ion-label>
        </ion-item>
        <ion-item v-if="isTorrent">
          <ion-label>
            <p>Uploaded</p>
            <h2>{{ formatBytes(task.uploaded) }}</h2>
          </ion-label>
        </ion-item>
        <ion-item>
          <ion-label>
            <p>{{ isTorrent ? 'Speeds' : 'Speed' }}</p>
            <h2>
              ↓ {{ formatSpeed(task.downloadSpeed)
              }}<template v-if="isTorrent"> · ↑ {{ formatSpeed(task.uploadSpeed) }}</template>
            </h2>
          </ion-label>
        </ion-item>
        <ion-item v-if="isTorrent">
          <ion-label>
            <p>Peers / seeders</p>
            <h2>{{ task.peers }} / {{ task.seeders }}</h2>
          </ion-label>
          <ion-note slot="end">{{ task.type }}</ion-note>
        </ion-item>
      </ion-list>
    </ion-content>
    <ion-footer v-if="canRedownload || canOpenInDiscover" :translucent="true">
      <ion-toolbar>
        <ion-button
          v-if="canOpenInDiscover"
          expand="block"
          fill="outline"
          class="footer-btn"
          data-testid="detail-open-discover"
          @click="openInDiscover"
        >
          <ion-icon slot="start" :icon="filmOutline" />
          Open in Discover
        </ion-button>
        <ion-button
          v-if="canRedownload"
          expand="block"
          class="footer-btn"
          data-testid="detail-redownload"
          @click="redownload"
        >
          <ion-icon slot="start" :icon="refreshOutline" />
          Re-download
        </ion-button>
      </ion-toolbar>
    </ion-footer>
  </ion-modal>
</template>

<style scoped>
.footer-btn {
  margin: 0.4rem 0.6rem;
}
.detail-progress-bar {
  margin-top: 8px;
  height: 6px;
  border-radius: 3px;
  overflow: hidden;
}
</style>

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
