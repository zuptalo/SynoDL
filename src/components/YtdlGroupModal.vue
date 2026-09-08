<script setup lang="ts">
/**
 * A playlist or channel's contents (spec 0013, US6, FR-019a).
 *
 * This exists because expansion has no ceiling. A channel of several hundred
 * items rendered flat into the Tasks list would push every other download —
 * including every NAS task — off the screen, so the group is ONE row there and
 * its items live behind it, here.
 *
 * Each item is a full download in its own right once opened: its own state, its
 * own progress, its own retry. That is the whole point of expanding rather than
 * treating a channel as one opaque job.
 */
import {
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonIcon,
  IonInfiniteScroll,
  IonInfiniteScrollContent,
  IonItem,
  IonLabel,
  IonList,
  IonModal,
  IonProgressBar,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { refreshOutline, trashOutline } from 'ionicons/icons';
import { computed, ref, watch } from 'vue';
import { api, type YtdlDownload } from '@/services/api';

const props = defineProps<{ isOpen: boolean; group: YtdlDownload | null }>();
const emit = defineEmits<{
  (e: 'dismiss'): void;
  (e: 'retry', requestId: string): void;
  (e: 'open', requestId: string): void;
}>();

const items = ref<YtdlDownload[]>([]);
const cursor = ref<string | undefined>(undefined);
const loading = ref(false);

/**
 * Items are fetched here rather than carried in the list payload, and paged.
 * With no ceiling on expansion a group can hold thousands, and sending them all
 * on every poll of the Tasks list would make the whole list slow for everyone —
 * including the people who never opened a group.
 */
async function load(reset: boolean): Promise<void> {
  const id = props.group?.requestId;
  if (!id || loading.value) return;
  loading.value = true;
  try {
    const page = await api.ytdlItems(id, reset ? undefined : cursor.value);
    items.value = reset ? page.items : [...items.value, ...page.items];
    cursor.value = page.nextCursor;
  } catch {
    // Leave whatever is already shown; the next open re-reads.
  } finally {
    loading.value = false;
  }
}

watch(
  () => [props.isOpen, props.group?.requestId],
  ([open]) => {
    if (open) {
      items.value = [];
      cursor.value = undefined;
      void load(true);
    }
  },
  { immediate: true },
);

const summary = computed(() => {
  const c = props.group?.counts;
  if (!c) return '';
  const parts = [`${c.completed} of ${c.total} saved`];
  if (c.failed > 0) parts.push(`${c.failed} failed`);
  if (c.remaining > 0) parts.push(`${c.remaining} to go`);
  return parts.join(' · ');
});

const stateLabels: Record<string, string> = {
  resolving: 'reading contents',
  queued: 'waiting its turn',
  scheduled: 'starting',
  downloading: 'downloading',
  completed: 'saved',
  failed: 'failed',
};

const stateColors: Record<string, string> = {
  resolving: 'var(--ion-color-medium)',
  queued: 'var(--ion-color-medium)',
  scheduled: 'var(--ion-color-medium)',
  downloading: 'var(--ion-color-primary)',
  completed: 'var(--ion-color-success)',
  failed: 'var(--ion-color-danger)',
};

function progressOf(d: YtdlDownload): number | undefined {
  return d.state === 'downloading' && d.progress !== undefined ? d.progress : undefined;
}
</script>

<template>
  <ion-modal :is-open="isOpen" @did-dismiss="$emit('dismiss')">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>{{ group?.title || 'Contents' }}</ion-title>
        <ion-buttons slot="end">
          <ion-button data-testid="ytdl-group-close" @click="$emit('dismiss')">Close</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content>
      <div v-if="summary" class="summary" data-testid="ytdl-group-summary-header">{{ summary }}</div>

      <ion-list data-testid="ytdl-group-items">
        <ion-item
          v-for="item in items"
          :key="item.requestId"
          button
          :detail="false"
          data-testid="ytdl-group-item"
          @click="emit('open', item.requestId)"
        >
          <ion-label class="ion-text-wrap">
            <h2 class="name">{{ item.title || item.url }}</h2>
            <div class="meta">
              <span :style="{ color: stateColors[item.state] }" data-testid="ytdl-group-item-status">
                {{ stateLabels[item.state] }}
              </span>
              <span v-if="item.reason" class="reason">{{ item.reason }}</span>
            </div>
            <ion-progress-bar
              v-if="progressOf(item) !== undefined"
              :value="progressOf(item)"
              :style="{ '--progress-background': stateColors[item.state] }"
            />
          </ion-label>
          <ion-button
            v-if="item.state === 'failed'"
            slot="end"
            fill="clear"
            data-testid="ytdl-group-item-retry"
            @click.stop="emit('retry', item.requestId)"
          >
            <ion-icon slot="icon-only" :icon="refreshOutline" />
          </ion-button>
        </ion-item>
      </ion-list>

      <div v-if="loading && items.length === 0" class="center"><ion-spinner name="crescent" /></div>
      <div v-else-if="!loading && items.length === 0" class="center empty">
        <!-- A group with nothing in it is a real answer: re-running a channel
             you already hold in full has nothing left to add. -->
        <p>Nothing new to download here.</p>
      </div>

      <ion-infinite-scroll
        v-if="cursor"
        @ion-infinite="load(false).then(() => ($event.target as HTMLIonInfiniteScrollElement).complete())"
      >
        <ion-infinite-scroll-content />
      </ion-infinite-scroll>
    </ion-content>
  </ion-modal>
</template>

<style scoped>
.summary {
  padding: 12px 16px;
  color: var(--ion-color-medium);
  font-size: 0.9rem;
}
.name {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 0.8rem;
  color: var(--ion-color-medium);
  margin-top: 2px;
}
.reason {
  overflow: hidden;
  text-overflow: ellipsis;
}
.center {
  display: flex;
  justify-content: center;
  padding: 32px;
}
.empty {
  color: var(--ion-color-medium);
}
ion-progress-bar {
  margin-top: 6px;
  height: 3px;
  border-radius: 2px;
}
</style>
