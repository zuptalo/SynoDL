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
  IonInfiniteScroll,
  IonInfiniteScrollContent,
  IonList,
  IonModal,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { computed, ref, watch } from 'vue';
import { api, type YtdlDownload } from '@/services/api';
import YtdlItem from '@/components/YtdlItem.vue';

const props = defineProps<{ isOpen: boolean; group: YtdlDownload | null }>();
const emit = defineEmits<{
  (e: 'dismiss'): void;
  (e: 'retry', requestId: string): void;
  (e: 'open', requestId: string): void;
  (e: 'remove', requestId: string): void;
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
    // Leave whatever is already shown; the next refresh re-reads.
  } finally {
    loading.value = false;
  }
}

/**
 * Update what is already on screen, in place.
 *
 * Deliberately NOT `load(true)`. Re-reading the first page and assigning it
 * would blank the list and repopulate it — which is visible as a flicker every
 * few seconds, and throws away any further pages the reader has scrolled to.
 * Only the fields that actually move are copied onto the rows already shown.
 */
async function refreshInPlace(): Promise<void> {
  const id = props.group?.requestId;
  if (!id || loading.value || items.value.length === 0) return;
  try {
    const page = await api.ytdlItems(id);
    const fresh = new Map(page.items.map((i) => [i.requestId, i]));
    items.value = items.value.map((existing) => fresh.get(existing.requestId) ?? existing);
  } catch {
    // Keep showing what we have.
  }
}

/**
 * Two SEPARATE sources, not one getter returning an array.
 *
 * A getter that builds a new array is a new object every time it runs, so Vue's
 * reference comparison fires the watcher on every re-render — and since `group`
 * comes from the polled list, that meant clearing and reloading the items every
 * few seconds. An array of sources is compared element-wise, so this fires only
 * when the sheet opens or when it is pointed at a different group.
 */
watch(
  [() => props.isOpen, () => props.group?.requestId],
  ([open]) => {
    if (open) {
      items.value = [];
      cursor.value = undefined;
      void load(true);
    }
  },
  { immediate: true },
);

/**
 * While the sheet is open, follow the group's own updates.
 *
 * Watched as a STRING, not as the counts object. The parent re-reads the list on
 * its own schedule, so `group.counts` is a brand new object every few seconds
 * even when not one number in it has changed — and an object source, deep or
 * not, fires on that new reference. Comparing a derived string compares by
 * value, so this runs when something actually moved and not merely when the
 * list was polled.
 */
watch(
  () => {
    const g = props.group;
    if (!g) return '';
    const c = g.counts;
    return c ? `${g.state}:${c.total}/${c.completed}/${c.failed}/${c.remaining}` : g.state;
  },
  () => {
    if (props.isOpen) void refreshInPlace();
  },
);

const summary = computed(() => {
  const c = props.group?.counts;
  if (!c) return '';
  const parts = [`${c.completed} of ${c.total} saved`];
  if (c.failed > 0) parts.push(`${c.failed} failed`);
  if (c.remaining > 0) parts.push(`${c.remaining} to go`);
  return parts.join(' · ');
});

// No label, colour or progress maps here on purpose: a track is rendered by the
// SAME row component as a top-level download (FR-003), so the two cannot drift
// apart the way they already had — the tracks had lost artwork, the source
// marker, the artist and the mode.
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
        <!-- The SAME row as the Tasks list. A track is a download in every
             respect the reader cares about, so it is rendered by the same
             component rather than by a thinner copy of it (FR-003, FR-004). -->
        <YtdlItem
          v-for="item in items"
          :key="item.requestId"
          :download="item"
          data-testid="ytdl-group-item"
          @open="emit('open', $event)"
          @retry="emit('retry', $event)"
          @dismiss="emit('remove', $event)"
        />
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
/* Room to breathe (spec 1037, FR-008).
   Ionic's list items sit flush to the edge on a full-screen sheet, which on a
   phone puts text hard against the bezel. The inset is applied to the CONTENT
   rather than to each item so every row lines up, and the bottom clears the home
   indicator via the safe-area inset rather than a guessed constant. */
ion-content {
  --padding-start: 8px;
  --padding-end: 8px;
  --padding-top: 4px;
  --padding-bottom: calc(16px + var(--ion-safe-area-bottom, 0px));
}

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
