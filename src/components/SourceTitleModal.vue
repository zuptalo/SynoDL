<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import {
  alertController,
  IonBadge,
  IonButton,
  IonButtons,
  IonCheckbox,
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
  IonSegment,
  IonSegmentButton,
  IonSpinner,
  IonThumbnail,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { arrowForwardOutline, cloudDownloadOutline } from 'ionicons/icons';
import { api, ApiError, posterSrc, type CatalogTitle, type QualityOption } from '@/services/api';
import { bySeasonThenSize, sizeMB } from '@/services/quality-sort';
import { appToast } from '@/services/toast';
import { useSourceCatalog } from '@/composables/useSourceCatalog';
import { splitYear } from '@/services/title-year';
import type { Task } from '@/types/task';

const props = defineProps<{
  isOpen: boolean;
  title: CatalogTitle;
}>();
// Clean title + separate release year for the header (the raw title is still what
// send() uses, so the created subfolder keeps the year).
const titleParts = computed(() => splitYear(props.title.title));
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

// ---- per-episode selection (series) ---------------------------------------
// The selected quality; for a series this is a season pack with an episode
// count. The user can pick a subset of episodes (1-based); by default all are
// selected. A movie has no episode count and this whole block stays hidden.
const selectedQuality = computed(() => qualities.value.find((q) => q.id === selected.value));
const episodeCount = computed(() => selectedQuality.value?.episodes ?? 0);
const isSeriesPack = computed(() => episodeCount.value > 1);
const selectedEpisodes = ref<number[]>([]);
const allEpisodes = computed(() => Array.from({ length: episodeCount.value }, (_, i) => i + 1));
const allEpisodesSelected = computed(
  () => episodeCount.value > 0 && selectedEpisodes.value.length === episodeCount.value,
);
// Reset the selection to "all episodes" whenever the chosen quality changes.
watch(selected, () => {
  selectedEpisodes.value = isSeriesPack.value ? allEpisodes.value.slice() : [];
});
function toggleEpisode(n: number, ev: CustomEvent): void {
  const on = (ev.detail as { checked: boolean }).checked;
  const set = new Set(selectedEpisodes.value);
  if (on) set.add(n);
  else set.delete(n);
  selectedEpisodes.value = [...set].sort((a, b) => a - b);
}
function toggleAllEpisodes(on: boolean): void {
  selectedEpisodes.value = on ? allEpisodes.value.slice() : [];
}
// The episodes argument to send: the picked subset for a series, or undefined
// for a movie / the whole season (server treats empty as everything).
function episodeArg(): number[] | undefined {
  if (!isSeriesPack.value) return undefined;
  if (selectedEpisodes.value.length === 0 || allEpisodesSelected.value) return undefined;
  return selectedEpisodes.value.slice();
}
// How many downloads this send will start (episodes, or 1 for a movie).
const sendCount = computed(() => {
  if (!isSeriesPack.value) return 1;
  return selectedEpisodes.value.length || episodeCount.value;
});

// ---- daily download allowance ---------------------------------------------
const quota = ref<{ limit: number; used: number; remaining: number } | null>(null);
const remainingLabel = computed(() => {
  const q = quota.value;
  if (!q || q.limit <= 0) return '';
  return `${q.remaining} of ${q.limit} downloads left today`;
});
async function loadQuota(): Promise<void> {
  try {
    quota.value = await api.getSourceQuota();
  } catch {
    quota.value = null;
  }
}

// Instance-wide max download size (MB, 0 = unlimited) from the source status.
const maxMB = computed(() => status.value?.maxDownloadMB ?? 0);
const maxLabel = computed(() =>
  maxMB.value > 0 ? `${+(maxMB.value / 1024).toFixed(1)} GB` : '',
);

function tooLarge(q: QualityOption): boolean {
  return maxMB.value > 0 && sizeMB(q.size) > maxMB.value;
}
// Whether the currently-selected quality is over the size cap (blocks Send).
const selectedTooLarge = computed(() => {
  const q = qualities.value.find((x) => x.id === selected.value);
  return q ? tooLarge(q) : false;
});

// ---- quality tier tabs (4K / 1080p / 720p …) ------------------------------
// Group options by resolution tier so the user can jump between quality classes.
// The label's resolution token is the reliable signal — many encodes are cropped
// (e.g. 1920x1040), so the raw pixel height alone would misclassify them.
interface Tier {
  key: string;
  label: string;
  rank: number;
}
function tierOf(q: QualityOption): Tier {
  const s = `${q.label} ${q.resolution}`;
  if (/2160p|\buhd\b|\b4k\b|3840x/i.test(s)) return { key: '4k', label: '4K', rank: 4 };
  if (/1080p|1920x/i.test(s)) return { key: '1080', label: '1080p', rank: 3 };
  if (/720p|1280x/i.test(s)) return { key: '720', label: '720p', rank: 2 };
  if (/480p|854x|640x/i.test(s)) return { key: '480', label: '480p', rank: 1 };
  return { key: 'other', label: 'Other', rank: 0 };
}
// Distinct tiers present, highest quality first (the tab order).
const tiers = computed<Tier[]>(() => {
  const map = new Map<string, Tier>();
  for (const q of qualities.value) {
    const t = tierOf(q);
    if (!map.has(t.key)) map.set(t.key, t);
  }
  return [...map.values()].sort((a, b) => b.rank - a.rank);
});
const activeTier = ref('');
// The active tier's options: by season, then largest file first.
const visibleQualities = computed(() =>
  qualities.value.filter((q) => tierOf(q).key === activeTier.value).slice().sort(bySeasonThenSize),
);
// The default sendable option in a tier (first usable in the display order — the
// earliest season's largest usable), else its first option.
function firstUsableIn(tierKey: string): string {
  const list = qualities.value
    .filter((q) => tierOf(q).key === tierKey)
    .slice()
    .sort(bySeasonThenSize);
  return (list.find((q) => !tooLarge(q)) ?? list[0])?.id ?? '';
}
function onTier(key: string): void {
  activeTier.value = key;
  // Keep the current pick if it's in this tier; else select its first usable.
  const cur = qualities.value.find((q) => q.id === selected.value);
  if (!cur || tierOf(cur).key !== key) selected.value = firstUsableIn(key);
}

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
      // Default to the highest-quality tab, selecting its first usable option
      // (largest that's within the size limit). The user's preferred quality, if
      // it lives in that top tier, wins over the plain first.
      const topTier = tiers.value[0]?.key ?? '';
      activeTier.value = topTier;
      const inTop = detail.qualities
        .filter((q) => tierOf(q).key === topTier && !tooLarge(q))
        .slice()
        .sort(bySeasonThenSize);
      const preferred = inTop.find((q) =>
        preferredQuality.value ? q.label.toLowerCase().includes(preferredQuality.value.toLowerCase()) : false,
      );
      selected.value = preferred?.id ?? firstUsableIn(topTier);
      void loadQuota(); // show the daily allowance alongside the qualities
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
  await appToast({ message, duration: 3000 });
}

async function send(episodesOverride?: number[]): Promise<void> {
  if (!selected.value || sending.value) return;
  // A series with nothing ticked has nothing to send.
  if (isSeriesPack.value && !episodesOverride && selectedEpisodes.value.length === 0) return;
  sending.value = true;
  errorMsg.value = '';
  try {
    const episodes = episodesOverride ?? episodeArg();
    const res = await api.sendSource(
      props.title.id,
      selected.value,
      props.title.title,
      props.title.type,
      episodes,
      { year: titleParts.value.year, imdbScore: props.title.imdbScore, posterUrl: props.title.posterUrl },
    );
    // Stay open and flip the button into a live status control; poll the task
    // list for the download(s) we just created so the button tracks their state.
    sent.value = true;
    sentDest.value = res.destination;
    startPolling();
    void loadQuota(); // reflect the allowance we just used
    await toast(res.count > 1 ? `Sent ${res.count} episodes to your NAS` : 'Sent to your NAS');
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
      } else if (e.code === 'daily_limit_exceeded' || e.code === 'daily_limit_reached') {
        await offerOverLimit();
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

// When a send would exceed the daily allowance, offer to send just what fits
// (the first N episodes) or cancel and pick fewer.
async function offerOverLimit(): Promise<void> {
  await loadQuota();
  const remaining = quota.value?.remaining ?? 0;
  const buttons: Parameters<typeof alertController.create>[0]['buttons'] = [];
  if (remaining > 0) {
    buttons.push({
      text: `Send first ${remaining}`,
      handler: () => {
        const base = selectedEpisodes.value.length ? selectedEpisodes.value : allEpisodes.value;
        void send(base.slice(0, remaining));
      },
    });
  }
  buttons.push({ text: remaining > 0 ? 'Pick fewer' : 'OK', role: 'cancel' });
  const alert = await alertController.create({
    header: 'Daily download limit',
    message:
      remaining > 0
        ? `You can start ${remaining} more download${remaining === 1 ? '' : 's'} today, but this is ${sendCount.value} episodes. Send the first ${remaining}, or pick fewer and try again.`
        : "You've used today's download allowance. Try again tomorrow, or ask an admin to reset your count.",
    buttons,
  });
  await alert.present();
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
            <h2>{{ titleParts.title }}</h2>
            <p class="meta">
              <span class="type">{{ title.type }}</span>
              <span v-if="titleParts.year" class="year">{{ titleParts.year }}</span>
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
          <!-- Quality tiers as tabs (highest first); scrolls when there are many. -->
          <ion-segment
            v-if="tiers.length > 1"
            :value="activeTier"
            scrollable
            class="tier-tabs"
            @ionChange="(e) => onTier(String(e.detail.value))"
          >
            <ion-segment-button v-for="t in tiers" :key="t.key" :value="t.key">
              <ion-label>{{ t.label }}</ion-label>
            </ion-segment-button>
          </ion-segment>
          <ion-radio-group v-model="selected">
            <ion-list :inset="true">
              <ion-item v-for="q in visibleQualities" :key="q.id">
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

          <!-- Series: pick which episodes to download (all by default). -->
          <template v-if="!sent && isSeriesPack">
            <div class="ep-head">
              <span class="ep-title">Episodes ({{ selectedEpisodes.length }}/{{ episodeCount }})</span>
              <ion-button
                size="small"
                fill="clear"
                data-testid="ep-select-all"
                @click="toggleAllEpisodes(!allEpisodesSelected)"
              >
                {{ allEpisodesSelected ? 'Clear all' : 'Select all' }}
              </ion-button>
            </div>
            <ion-list :inset="true" class="ep-list">
              <ion-item v-for="n in allEpisodes" :key="n">
                <ion-checkbox
                  :checked="selectedEpisodes.includes(n)"
                  label-placement="end"
                  justify="start"
                  @ion-change="(e) => toggleEpisode(n, e)"
                >
                  Episode {{ n }}
                </ion-checkbox>
              </ion-item>
            </ion-list>
          </template>

          <ion-note v-if="!sent && remainingLabel" color="medium" class="cap-hint" data-testid="quota-hint">
            {{ remainingLabel }}
          </ion-note>
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
          :disabled="!selected || sending || selectedTooLarge || (isSeriesPack && selectedEpisodes.length === 0)"
          @click="send()"
        >
          <ion-spinner v-if="sending" slot="start" name="crescent" />
          <ion-icon v-else slot="start" :icon="cloudDownloadOutline" />
          {{ isSeriesPack ? `Send ${sendCount} to NAS` : 'Send to NAS' }}
        </ion-button>
      </template>
    </ion-content>
  </ion-modal>
</template>

<style scoped>
.ep-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 1rem;
  margin-top: 4px;
}
.ep-title {
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--app-text-dim);
}
.ep-list {
  max-height: 40vh;
  overflow-y: auto;
}
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
.tier-tabs {
  margin: 2px 8px 8px;
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
