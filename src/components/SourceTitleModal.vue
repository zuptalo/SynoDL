<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import {
  IonBadge,
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonIcon,
  IonItem,
  IonLabel,
  IonList,
  IonModal,
  IonNote,
  IonRadio,
  IonRadioGroup,
  IonSpinner,
  IonThumbnail,
  IonTitle,
  IonToolbar,
  toastController,
} from '@ionic/vue';
import { arrowForwardOutline, cloudDownloadOutline } from 'ionicons/icons';
import { api, ApiError, posterSrc, type CatalogTitle, type QualityOption } from '@/services/api';
import { useSourceCatalog } from '@/composables/useSourceCatalog';
import type { Task } from '@/types/task';

const props = defineProps<{
  isOpen: boolean;
  title: CatalogTitle;
}>();
const posterFailed = ref(false);
// Two-stage poster load: try posterUrl, then the reliable fallback, then the
// letter tile — so a title whose cover URL 404s still shows its placeholder art.
const posterFellBack = ref(false);
const posterSource = computed(() =>
  posterFellBack.value ? posterSrc(props.title.posterFallbackUrl ?? '') : posterSrc(props.title.posterUrl),
);
function onPosterError(): void {
  if (
    !posterFellBack.value &&
    props.title.posterFallbackUrl &&
    props.title.posterFallbackUrl !== props.title.posterUrl
  ) {
    posterFellBack.value = true;
    return;
  }
  posterFailed.value = true;
}

// The wide backdrop shown large behind the header. Falls back to nothing (the
// header just shows the poster) if the title has no distinct cover or it fails.
const backdropFailed = ref(false);
const heroSrc = computed(() =>
  props.title.backdropUrl && !backdropFailed.value ? posterSrc(props.title.backdropUrl) : '',
);
const emit = defineEmits<{
  (e: 'dismiss'): void;
  (e: 'needs-refresh'): void;
}>();

const { preferredQuality, status } = useSourceCatalog();
const router = useRouter();

const loading = ref(false);
const sending = ref(false);
const sendable = ref(true);
const qualities = ref<QualityOption[]>([]);
const selected = ref('');
const errorMsg = ref('');

// Instance-wide max download size (MB, 0 = unlimited) from the source status.
const maxMB = computed(() => status.value?.maxDownloadMB ?? 0);
const maxLabel = computed(() =>
  maxMB.value > 0 ? `${+(maxMB.value / 1024).toFixed(1)} GB` : '',
);

// Parse a provider size string ("11 GB") into MB; 0 when unknown.
function sizeMB(size: string): number {
  const m = /([\d.]+)\s*(TB|GB|MB|KB)/i.exec(size);
  if (!m) return 0;
  const v = parseFloat(m[1]);
  const unit = m[2].toUpperCase();
  return Math.round(unit === 'TB' ? v * 1024 * 1024 : unit === 'GB' ? v * 1024 : unit === 'KB' ? v / 1024 : v);
}
function tooLarge(q: QualityOption): boolean {
  return maxMB.value > 0 && sizeMB(q.size) > maxMB.value;
}
// Whether the currently-selected quality is over the size cap (blocks Send).
const selectedTooLarge = computed(() => {
  const q = qualities.value.find((x) => x.id === selected.value);
  return q ? tooLarge(q) : false;
});

// ---- post-send live state -------------------------------------------------
// After a successful send we don't dismiss; the Send button becomes a live
// status button that polls the task list for the download(s) we just created
// (matched by their destination subfolder) and links through to the task detail.
const sent = ref(false);
const sentDest = ref('');
const liveTasks = ref<Task[]>([]);
let pollTimer: ReturnType<typeof setInterval> | null = null;

// The task's `destination` is share-relative (e.g. "movies/Spider-Man"); match on
// the full path, or fall back to the leaf folder we created for this title.
function sameDest(taskDest: string, dest: string): boolean {
  const norm = (s: string): string => s.replace(/^\/+/, '').replace(/\/+$/, '');
  const a = norm(taskDest);
  const b = norm(dest);
  if (!a || !b) return false;
  const folder = b.split('/').pop() ?? '';
  return a === b || (folder !== '' && a.endsWith('/' + folder));
}
const sentTasks = computed(() =>
  sentDest.value ? liveTasks.value.filter((t) => sameDest(t.destination, sentDest.value)) : [],
);
const primaryTask = computed<Task | null>(() => sentTasks.value[0] ?? null);

const STATUS_LABEL: Record<string, string> = {
  waiting: 'Queued',
  filehosting_waiting: 'Waiting',
  downloading: 'Downloading',
  paused: 'Paused',
  finishing: 'Finishing',
  finished: 'Completed',
  seeding: 'Seeding',
  hash_checking: 'Checking',
  extracting: 'Extracting',
  error: 'Error',
};
const sentLabel = computed(() => {
  if (sentTasks.value.length > 1) return `${sentTasks.value.length} downloads · View in Tasks`;
  const t = primaryTask.value;
  if (!t) return 'Added to NAS · View in Tasks';
  const base = STATUS_LABEL[t.status] ?? t.status;
  if (t.size > 0 && t.status === 'downloading') {
    return `${base} · ${Math.min(100, Math.floor((t.downloaded / t.size) * 100))}%`;
  }
  return `${base} · View download`;
});

async function pollSent(): Promise<void> {
  try {
    liveTasks.value = (await api.tasks()).tasks;
  } catch {
    // Leave the button in its last state; the poll retries on the next tick.
  }
}
function startPolling(): void {
  if (pollTimer) return;
  void pollSent();
  pollTimer = setInterval(() => void pollSent(), 3000);
}
function stopPolling(): void {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}
onUnmounted(stopPolling);

function viewDownload(): void {
  const t = primaryTask.value;
  emit('dismiss');
  // A single created task deep-links to its detail; a season pack (many tasks)
  // just lands on the Tasks list.
  if (t && sentTasks.value.length === 1) {
    void router.push({ path: '/tabs/tasks', query: { task: t.id } });
  } else {
    void router.push('/tabs/tasks');
  }
}

watch(
  () => props.isOpen,
  async (open) => {
    if (!open) {
      stopPolling();
      return;
    }
    // Reset any prior send state so a reopened title starts fresh.
    sent.value = false;
    sentDest.value = '';
    liveTasks.value = [];
    stopPolling();
    loading.value = true;
    errorMsg.value = '';
    qualities.value = [];
    selected.value = '';
    posterFailed.value = false;
    posterFellBack.value = false;
    backdropFailed.value = false;
    try {
      const detail = await api.getSourceTitle(props.title.id);
      sendable.value = detail.sendable;
      qualities.value = detail.qualities;
      // Preselect the preferred quality among those within the size limit,
      // otherwise the first usable one.
      const usable = detail.qualities.filter((q) => !tooLarge(q));
      const preferred = usable.find((q) =>
        preferredQuality.value ? q.label.toLowerCase().includes(preferredQuality.value.toLowerCase()) : false,
      );
      selected.value = preferred?.id ?? usable[0]?.id ?? '';
    } catch (e) {
      if (e instanceof ApiError && e.code === 'source_needs_refresh') {
        emit('needs-refresh');
        emit('dismiss');
        return;
      }
      errorMsg.value = 'Could not load this title.';
    } finally {
      loading.value = false;
    }
  },
  // Fire on mount too: the modal is mounted with is-open already true (the parent
  // sets the title and opens it in the same tick), so without `immediate` the
  // very first open would never load — the bug where options only appeared after
  // closing and reopening.
  { immediate: true },
);

async function toast(message: string): Promise<void> {
  // A plain confirmation — the send button itself becomes the live "view the
  // download" affordance, so the toast no longer needs its own action.
  const t = await toastController.create({
    message,
    duration: 3000,
    position: 'top',
    cssClass: 'app-toast',
    swipeGesture: 'vertical',
  });
  await t.present();
}

async function send(): Promise<void> {
  if (!selected.value || sending.value) return;
  sending.value = true;
  errorMsg.value = '';
  try {
    const res = await api.sendSource(props.title.id, selected.value, props.title.title, props.title.type);
    // Stay open and flip the button into a live status control; poll the task
    // list for the download(s) we just created so the button tracks their state.
    sent.value = true;
    sentDest.value = res.destination;
    startPolling();
    await toast('Sent to your NAS');
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.code === 'source_needs_refresh') {
        emit('needs-refresh');
        emit('dismiss');
        return;
      }
      if (e.code === 'destination_forbidden') {
        errorMsg.value = "You can't download to that folder.";
      } else if (e.code === 'download_too_large') {
        errorMsg.value = maxLabel.value
          ? `The admin set a max download size of ${maxLabel.value}. Pick a smaller quality.`
          : 'That download exceeds the admin size limit. Pick a smaller quality.';
      } else if (e.code === 'daily_limit_reached') {
        errorMsg.value = "You've reached your daily download limit set by the admin. Try again later.";
      } else if (e.code === 'send_failed') {
        errorMsg.value = 'The download link could not be used. Try again.';
      } else {
        errorMsg.value = 'Could not send to the NAS.';
      }
    } else {
      errorMsg.value = 'Could not send to the NAS.';
    }
  } finally {
    sending.value = false;
  }
}
</script>

<template>
  <ion-modal :is-open="isOpen" @didDismiss="emit('dismiss')">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>Details</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="emit('dismiss')">Close</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content class="ion-padding">
      <div v-if="loading" class="centered"><ion-spinner /></div>

      <template v-else>
        <!-- Wide backdrop shown large behind the header; the poster overlaps its
             lower edge. Absent for titles with no distinct cover. -->
        <div v-if="heroSrc" class="hero">
          <img class="hero-bg" :src="heroSrc" alt="" aria-hidden="true" @error="backdropFailed = true" />
          <div class="hero-shade" />
        </div>
        <div class="poster-row" :class="{ 'on-hero': heroSrc }">
          <ion-thumbnail class="poster">
            <img
              v-if="(title.posterUrl || title.posterFallbackUrl) && !posterFailed"
              :src="posterSource"
              :alt="title.title"
              @error="onPosterError"
            />
            <div v-else class="poster-fallback">{{ title.title.charAt(0) }}</div>
          </ion-thumbnail>
          <div class="head">
            <h2>{{ title.title }}</h2>
            <p class="meta">
              <span class="type">{{ title.type }}</span>
              <span v-if="title.imdbScore">★ {{ title.imdbScore.toFixed(1) }} IMDb</span>
              <span v-if="title.providerScore">{{ title.providerScore.toFixed(1) }} 30N</span>
            </p>
            <p v-if="title.genres?.length" class="genres">
              {{ title.genres.slice(0, 4).join(' · ') }}
            </p>
          </div>
        </div>

        <p v-if="title.plot" class="plot">{{ title.plot }}</p>

        <ion-note v-if="!sendable" color="medium" class="unavailable">
          No downloadable files for this title yet.
        </ion-note>

        <template v-else>
          <ion-note v-if="maxLabel" color="medium" class="cap-hint">
            Max download size {{ maxLabel }} (set by admin). Options over it are marked and can't be sent.
          </ion-note>
          <ion-radio-group v-model="selected">
            <ion-list :inset="true">
              <ion-item v-for="q in qualities" :key="q.id">
                <ion-radio :value="q.id" label-placement="end" justify="start">
                  <ion-label>
                    <h3>
                      <span v-if="q.season" class="season">{{ q.season }} · </span>{{ q.label }}
                      <ion-badge v-if="tooLarge(q)" color="warning" class="too-large">Too large</ion-badge>
                    </h3>
                    <p>
                      {{ q.size }}<template v-if="q.resolution"> · {{ q.resolution }}</template
                      >{{ q.encoder ? ' · ' + q.encoder : ''
                      }}<template v-if="q.episodes"> · {{ q.episodes }} eps</template>
                    </p>
                  </ion-label>
                </ion-radio>
              </ion-item>
            </ion-list>
          </ion-radio-group>
        </template>

        <ion-note v-if="!sent && selectedTooLarge && !errorMsg" color="warning" class="error">
          That's over the {{ maxLabel }} limit — pick a smaller quality.
        </ion-note>
        <ion-note v-if="errorMsg" color="danger" class="error">{{ errorMsg }}</ion-note>

        <!-- After a successful send the button tracks the created download and
             links through to it, instead of the modal just closing. -->
        <ion-button
          v-if="sent"
          expand="block"
          class="send-btn"
          color="success"
          @click="viewDownload"
        >
          <ion-icon slot="start" :icon="arrowForwardOutline" />
          {{ sentLabel }}
        </ion-button>
        <ion-button
          v-else-if="sendable"
          expand="block"
          class="send-btn"
          :disabled="!selected || sending || selectedTooLarge"
          @click="send"
        >
          <ion-spinner v-if="sending" slot="start" name="crescent" />
          <ion-icon v-else slot="start" :icon="cloudDownloadOutline" />
          Send to NAS
        </ion-button>
      </template>
    </ion-content>
  </ion-modal>
</template>

<style scoped>
.centered {
  display: flex;
  justify-content: center;
  padding-top: 30vh;
}
/* Full-bleed backdrop: break out of the content's ion-padding to the top/sides. */
.hero {
  position: relative;
  margin: -16px -16px 0;
  height: 180px;
  overflow: hidden;
}
.hero-bg {
  width: 100%;
  height: 100%;
  object-fit: cover;
  filter: brightness(0.62);
}
/* Fade the backdrop into the page so the poster/title sit on solid colour. */
.hero-shade {
  position: absolute;
  inset: 0;
  background: linear-gradient(
    to bottom,
    rgba(0, 0, 0, 0) 35%,
    var(--ion-background-color, #0a0a0a) 100%
  );
}
.poster-row {
  display: flex;
  gap: 12px;
  align-items: center;
  margin-bottom: 8px;
}
/* With a backdrop, tuck the header up into the hero's faded lower edge (which is
   ~background colour there, so the text stays readable in both themes). */
.poster-row.on-hero {
  margin-top: -34px;
  position: relative;
  z-index: 1;
}
.poster {
  --size: 96px;
  flex: 0 0 auto;
}
.poster img {
  object-fit: cover;
  border-radius: 8px;
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.4);
}
.poster-row h2 {
  margin: 0 0 4px;
  font-size: 1.1rem;
}
.poster-fallback {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.8rem;
  color: var(--app-text-dim);
  background: var(--ion-color-step-100, #1c1c1e);
  border-radius: 8px;
}
.head .meta {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin: 0 0 4px;
  font-size: 0.82rem;
  color: var(--app-text-dim);
}
.head .type {
  text-transform: capitalize;
}
.head .genres {
  margin: 0;
  font-size: 0.8rem;
  color: var(--app-text-dim);
  text-transform: capitalize;
}
.plot {
  margin: 4px 4px 12px;
  font-size: 0.9rem;
  line-height: 1.4;
  color: var(--app-text);
}
.unavailable {
  display: block;
  padding: 24px 8px;
  text-align: center;
}
.cap-hint {
  display: block;
  padding: 0 8px 6px;
}
.too-large {
  margin-left: 8px;
  vertical-align: middle;
  font-size: 0.7rem;
}
.season {
  color: var(--ion-color-primary, #3dc2ff);
  font-weight: 600;
}
.error {
  display: block;
  margin: 8px 4px;
}
.send-btn {
  margin-top: 16px;
}
</style>
