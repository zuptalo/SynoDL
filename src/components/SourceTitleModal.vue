<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import {
  alertController,
  IonAccordion,
  IonAccordionGroup,
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
  IonRadioGroup,
  IonSegment,
  IonSegmentButton,
  IonSpinner,
  IonThumbnail,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import SourceQualityRow from '@/components/SourceQualityRow.vue';
import { arrowForwardOutline, chevronBackOutline, cloudDownloadOutline, openOutline } from 'ionicons/icons';
import {
  api,
  ApiError,
  posterSrc,
  type CatalogTitle,
  type QualityOption,
  type SeasonPresence,
} from '@/services/api';
import { bySeasonThenSize, seasonNum, sizeMB } from '@/services/quality-sort';
import { useSourceCatalog } from '@/composables/useSourceCatalog';
import { splitYear } from '@/services/title-year';
import { imdbUrl } from '@/services/imdb-link';
import type { Task } from '@/types/task';

const props = defineProps<{
  isOpen: boolean;
  title: CatalogTitle;
  /**
   * Read-only mode, used by "Open in Discover" from a task: the download already
   * exists, so this is the title's information page — same header, backdrop,
   * genres and synopsis as Discover, but no qualities and no way to send. The
   * only actions are the header's back link and the large Dismiss button.
   */
  infoOnly?: boolean;
}>();

// The metadata actually rendered. Normally that's the prop as handed over by the
// Discover grid. The Tasks handoff can only build a stub from what the download
// row stored (id, title, poster, IMDb score), so `enriched` holds the full
// catalog entry once we've looked it up — see loadMeta().
const enriched = ref<CatalogTitle | null>(null);
// Metadata the SOURCE returned with the download options. Sources whose listing
// pages carry no synopsis and no IMDb link (ZarFilm) describe the title only on
// its own page, which the title request fetches anyway — see spec 1023.
const detailMeta = ref<{ imdbId?: string; plot?: string }>({});
// The catalog entry always wins: a source that puts a full English synopsis in
// its search results must never have it replaced by a thinner one from a detail
// page. Detail metadata fills gaps, nothing more (FR-008).
const info = computed<CatalogTitle>(() => {
  const base = enriched.value ?? props.title;
  return {
    ...base,
    imdbId: base.imdbId || detailMeta.value.imdbId || '',
    plot: base.plot || detailMeta.value.plot || '',
  };
});

// Clean title + separate release year for the header (the raw title is still what
// send() uses, so the created subfolder keeps the year).
const titleParts = computed(() => splitYear(info.value.title));
// Link to the title on IMDb when the provider gave us a usable id — "" means we
// render the rating as plain text instead (spec 1019). Opened with target=_blank,
// which an installed PWA hands to the device's browser; rel="noopener" keeps the
// opened page from reaching back into the app.
const imdbHref = computed(() => imdbUrl(info.value.imdbId));
const posterFailed = ref(false);
// Two-stage poster load: try posterUrl, then the reliable fallback, then the
// letter tile — so a title whose cover URL 404s still shows its placeholder art.
const posterFellBack = ref(false);
const posterSource = computed(() =>
  posterFellBack.value ? posterSrc(info.value.posterFallbackUrl ?? '') : posterSrc(info.value.posterUrl),
);
function onPosterError(): void {
  if (
    !posterFellBack.value &&
    info.value.posterFallbackUrl &&
    info.value.posterFallbackUrl !== info.value.posterUrl
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
  info.value.backdropUrl && !backdropFailed.value ? posterSrc(info.value.backdropUrl) : '',
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
// Which seasons are already on the NAS, and which episodes each holds. Empty
// when the title is a movie, is still downloading, or its folder could not be
// read — in every one of those cases no marker is shown rather than a wrong one.
const presentSeasons = ref<SeasonPresence[]>([]);
// Whether the NAS already has this title at all (FR-019a).
const ownership = ref<string>('unknown');
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

// ---- metadata lookup (Tasks handoff) --------------------------------------
// A download row remembers only the few catalog columns it needs to label
// itself, and the title endpoint returns download options rather than metadata —
// so the synopsis, genres, backdrop and IMDb id exist only in catalog search
// results. Look the stored title up and adopt the entry with our catalog id, so
// "Open in Discover" lands on the same page Discover itself shows. A miss (or a
// failed search) simply leaves the stub on screen; nothing here is fatal.
async function loadMeta(): Promise<void> {
  const stub = props.title;
  if (stub.plot || stub.genres?.length) return; // already a full catalog entry
  try {
    const res = await api.searchSource(splitYear(stub.title).title, {}, 1, '', '');
    const hit = res.items.find((i) => i.id === stub.id);
    if (hit) enriched.value = hit;
  } catch {
    /* keep the stub — the header just stays sparse */
  }
}

/** Metadata from the title's own page, for sources that publish it nowhere else. */
async function loadDetailMeta(): Promise<void> {
  try {
    const d = await api.getSourceTitle(props.title.id);
    detailMeta.value = { imdbId: d.imdbId, plot: d.plot };
  } catch {
    /* the synopsis is a bonus here, never the reason the sheet was opened */
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
/** Seasons already here, by number, for a per-option lookup. */
const seasonHave = computed(() => {
  const m = new Map<number, SeasonPresence>();
  for (const s of presentSeasons.value) m.set(s.season, s);
  return m;
});

/**
 * What to say about a season the user already has.
 *
 * Never "complete" and never "n of m": the catalog's episode count cannot be
 * relied on, so claiming a season is finished would assert more than the files
 * support (FR-016a). Listing the episode numbers says exactly what is there and
 * lets the user see the gap themselves.
 */
function haveLabel(q: QualityOption): string {
  const s = seasonHave.value.get(seasonNum(q));
  if (!s) return '';
  if (s.episodes.length === 0) return `On your NAS · ${s.videoFiles} file${s.videoFiles === 1 ? '' : 's'}`;
  return `On your NAS · ep ${s.episodes.join(', ')}`;
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

/**
 * The visible options grouped by season, for the accordion.
 *
 * A long pack list used to open fully expanded, every season at once, so a user
 * collecting a series scrolled past the seasons they already had to reach the one
 * they wanted (spec 1025 US2).
 */
interface SeasonGroup {
  key: string;
  season: number;
  label: string;
  options: QualityOption[];
  /** What is already on the NAS for this season, if anything. */
  present?: SeasonPresence;
}
// The active tier's options: by season, then largest file first. Seasons used to
// be separated by a divider here; they are now their own accordion groups.
const visibleQualities = computed(() =>
  qualities.value.filter((q) => tierOf(q).key === activeTier.value).slice().sort(bySeasonThenSize),
);
const seasonGroups = computed<SeasonGroup[]>(() => {
  const out: SeasonGroup[] = [];
  const byKey = new Map<number, SeasonGroup>();
  for (const q of visibleQualities.value) {
    const n = seasonNum(q);
    let g = byKey.get(n);
    if (!g) {
      g = {
        key: String(n),
        season: n,
        label: q.season || `Season ${n}`,
        options: [],
        present: seasonHave.value.get(n),
      };
      byKey.set(n, g);
      out.push(g);
    }
    g.options.push(q);
  }
  return out;
});

// Group only when there is a season to group BY. A movie's options have none, and
// wrapping them in a single accordion would add a layer that hides them behind a
// tap for no benefit (FR-011).
const groupedBySeason = computed(
  () => visibleQualities.value.some((q) => !!q.season) && seasonGroups.value.length > 0,
);

// Where "what do I do next" lives: the episode picker for a season pack, or the
// send button for a movie. Choosing a quality scrolls here, so the next step is
// on screen instead of below the fold of a long option list.
const downloadAnchor = ref<HTMLElement | null>(null);

watch(selected, (id, was) => {
  if (!id || id === was) return;
  void nextTick(() => {
    downloadAnchor.value?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  });
});

/** Which group is open. Presentation only — it never touches `selected`. */
const openSeason = ref<string | null>(null);

/**
 * The season to open on arrival: the first one NOT already on the NAS, so the
 * common case — "I have 1 and 2, I want 3" — costs no taps. When every season is
 * already here there is nothing to fetch, so nothing is opened (FR-008).
 */
function defaultOpenSeason(): string | null {
  const missing = seasonGroups.value.find((g) => !g.present);
  return missing ? missing.key : null;
}

function onSeasonToggle(value: string | null | undefined): void {
  openSeason.value = value ?? null;
  // Same rule as the tier tabs: the selection must always be something on screen.
  const cur = qualities.value.find((q) => q.id === selected.value);
  if (cur && String(seasonNum(cur)) !== openSeason.value) selected.value = '';
}

/** What a collapsed header says, so the season can be judged without opening it. */
function seasonHeadline(g: SeasonGroup): string {
  const count = `${g.options.length} option${g.options.length === 1 ? '' : 's'}`;
  if (!g.present) return count;
  const eps = g.present.episodes?.length ? ` · ep ${g.present.episodes.join(', ')}` : '';
  return `On your NAS${eps} · ${count}`;
}

// Re-open the right season whenever the grouping changes — a different tier shows
// a different set of packs, and the first season missing from THAT set may differ.
watch(
  () => seasonGroups.value.map((g) => `${g.key}:${g.present ? 1 : 0}`).join('|'),
  () => {
    openSeason.value = defaultOpenSeason();
  },
  { immediate: true },
);

function onTier(key: string): void {
  activeTier.value = key;
  // Keep the current pick if it is in this tier; otherwise drop it. Carrying a
  // selection the user can no longer see is how the send button ended up armed
  // with something off-screen.
  const cur = qualities.value.find((q) => q.id === selected.value);
  if (!cur || tierOf(cur).key !== key) selected.value = '';
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
    presentSeasons.value = [];
    ownership.value = 'unknown';
    selected.value = '';
    posterFailed.value = false;
    posterFellBack.value = false;
    backdropFailed.value = false;
    enriched.value = null;
    // Read-only mode has no options to fetch — only the metadata the stub is
    // missing. Keep the spinner up for it so the header doesn't render sparse
    // and then visibly re-draw with the backdrop and synopsis.
    if (props.infoOnly) {
      await loadMeta();
      // The catalog lookup is enough for a source that publishes metadata in its
      // search results. For one that does not, the title's own page is the only
      // place the synopsis exists — so ask for it, but only once we know the
      // catalog could not supply it, and never let it fail the sheet.
      if (!info.value.plot && !info.value.imdbId) await loadDetailMeta();
      loading.value = false;
      return;
    }
    try {
      const detail = await api.getSourceTitle(props.title.id);
      sendable.value = detail.sendable;
      qualities.value = detail.qualities;
      presentSeasons.value = detail.seasons ?? [];
      ownership.value = detail.ownership ?? 'unknown';
      detailMeta.value = { imdbId: detail.imdbId, plot: detail.plot };
      // Open on the user's preferred quality tab where the title has one, else the
      // highest. NOTHING is selected: a pre-selected option reads as a choice the
      // user made, and on a part-owned series the pre-selection sat inside a
      // COLLAPSED season, arming the send button with a season they already had.
      // Picking is now always a deliberate act.
      const preferredTier = preferredQuality.value
        ? tiers.value.find((t) => t.label.toLowerCase().includes(preferredQuality.value.toLowerCase()))
        : undefined;
      activeTier.value = preferredTier?.key ?? tiers.value[0]?.key ?? '';
      selected.value = '';
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

/**
 * Ask before fetching something that is already here, or already on its way.
 *
 * Both cases get the prompt (FR-019a): neither is something the user needs to
 * send again, and the daily allowance and the NAS's bandwidth are real costs.
 * Cancelling sends nothing and consumes no allowance (FR-020); a title that is
 * genuinely absent never prompts at all (FR-021).
 */
async function confirmDuplicate(): Promise<boolean> {
  const picked = selectedQuality.value;
  const season = picked ? seasonHave.value.get(seasonNum(picked)) : undefined;
  // A season pack is judged by ITS season, never by the title. A series counts as
  // owned when any season is on the NAS, so testing the title asked "download it
  // again?" for season 3 of a show the user had seasons 1 and 2 of — the one case
  // where they are certainly not downloading it again.
  const perSeason = !!picked?.season;
  const already =
    ownership.value === 'downloading' || (perSeason ? !!season : ownership.value === 'owned');
  if (!already) return true;

  const what =
    ownership.value === 'downloading'
      ? 'This is downloading right now.'
      : season
        ? `You already have season ${season.season} — ${haveLabel(picked!).replace('On your NAS · ', '')}.`
        : 'You already have this.';
  const alert = await alertController.create({
    header: 'Download it again?',
    message: `${what} Downloading it again will use your allowance.`,
    buttons: [
      { text: 'Cancel', role: 'cancel' },
      { text: 'Download anyway', role: 'confirm' },
    ],
  });
  await alert.present();
  const { role } = await alert.onDidDismiss();
  return role === 'confirm';
}

async function send(episodesOverride?: number[]): Promise<void> {
  if (!selected.value || sending.value) return;
  // A series with nothing ticked has nothing to send.
  if (isSeriesPack.value && !episodesOverride && selectedEpisodes.value.length === 0) return;
  if (!(await confirmDuplicate())) return;
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
      {
        year: titleParts.value.year,
        imdbScore: props.title.imdbScore,
        posterUrl: props.title.posterUrl,
        // Remember which version this is, so opening the title later can say it
        // is the one already on the NAS.
        qualityLabel: selectedQuality.value?.label,
        qualityResolution: selectedQuality.value?.resolution,
        qualityEncoder: selectedQuality.value?.encoder,
      },
    );
    // Stay open and flip the button into a live status control; poll the task
    // list for the download(s) we just created so the button tracks their state.
    sent.value = true;
    sentDest.value = res.destination;
    startPolling();
    void loadQuota(); // reflect the allowance we just used
    // No toast: the button under the user's finger has just become a live status
    // control for the very download they created, which says the same thing for
    // longer and in the place they are already looking.
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
        <!-- Read-only mode is reached from a task, and dismissing it lands on the
             Discover list underneath — so it reads as a back link rather than a
             modal's Close. -->
        <ion-buttons v-if="infoOnly" slot="start">
          <ion-button data-testid="title-back" @click="emit('dismiss')">
            <ion-icon slot="start" :icon="chevronBackOutline" />
            Discover
          </ion-button>
        </ion-buttons>
        <ion-buttons v-else slot="end">
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
              v-if="(info.posterUrl || info.posterFallbackUrl) && !posterFailed"
              :src="posterSource"
              :alt="info.title"
              @error="onPosterError"
            />
            <div v-else class="poster-fallback">{{ info.title.charAt(0) }}</div>
          </ion-thumbnail>
          <div class="head">
            <h2>{{ titleParts.title }}</h2>
            <p class="meta">
              <span class="type">{{ info.type }}</span>
              <span v-if="titleParts.year" class="year">{{ titleParts.year }}</span>
              <!-- The rating doubles as the way out to IMDb. With an id but no
                   score there's still a page worth visiting, so the link stands
                   on its own; with no id at all it stays plain text. -->
              <a
                v-if="imdbHref"
                class="imdb-link"
                :href="imdbHref"
                target="_blank"
                rel="noopener noreferrer"
                :aria-label="`Open ${titleParts.title} on IMDb`"
              >
                <template v-if="info.imdbScore">★ {{ info.imdbScore.toFixed(1) }} </template>IMDb
                <ion-icon :icon="openOutline" aria-hidden="true" />
              </a>
              <span v-else-if="info.imdbScore">★ {{ info.imdbScore.toFixed(1) }} IMDb</span>
              <span v-if="info.providerScore">{{ info.providerScore.toFixed(1) }} 30N</span>
            </p>
            <p v-if="info.genres?.length" class="genres">
              {{ info.genres.slice(0, 4).join(' · ') }}
            </p>
          </div>
        </div>

        <!-- dir="auto" rather than a fixed direction: the synopsis arrives in
             whatever language the source publishes (ZarFilm's is Persian), and the
             browser picks per string from its first strong character. Scoped to
             the paragraph, so the rest of the sheet keeps its own direction. -->
        <p v-if="info.plot" class="plot" dir="auto">{{ info.plot }}</p>

        <!-- Everything below is about starting a download, so read-only mode
             (opened from an existing task) stops here. -->
        <ion-button
          v-if="infoOnly"
          expand="block"
          color="medium"
          class="send-btn"
          data-testid="title-dismiss"
          @click="emit('dismiss')"
        >
          Dismiss
        </ion-button>

        <template v-else>
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
              <!-- No season to group by (a movie, or a source that lists releases
                   flat): the options are the list. -->
              <ion-list v-if="!groupedBySeason" :inset="true">
                <source-quality-row
                  v-for="q in visibleQualities"
                  :key="q.id"
                  :option="q"
                  :too-large="tooLarge(q)"
                  show-season
                />
              </ion-list>

              <!-- One season at a time. multiple=false gives the "opening one
                   closes the other" behaviour the user asked for, from Ionic
                   rather than from hand-rolled state (FR-009). -->
              <ion-accordion-group
                v-else
                class="season-groups"
                :value="openSeason"
                @ion-change="(e: CustomEvent) => onSeasonToggle(e.detail.value)"
              >
                <ion-accordion v-for="g in seasonGroups" :key="g.key" :value="g.key">
                  <ion-item slot="header" data-testid="season-group" :data-season="g.season">
                    <ion-label>
                      <h3 class="season">{{ g.label }}</h3>
                      <!-- Enough to judge the season without opening it: whether it
                           is here, which episodes, and how much is on offer. Which
                           episodes, never how many of a total — the total is not
                           something we can establish (FR-016a). -->
                      <p class="season-sub" :class="{ present: !!g.present }">
                        {{ seasonHeadline(g) }}
                      </p>
                    </ion-label>
                  </ion-item>
                  <div slot="content">
                    <ion-list :inset="false" class="season-options">
                      <source-quality-row
                        v-for="q in g.options"
                        :key="q.id"
                        :option="q"
                        :too-large="tooLarge(q)"
                      />
                    </ion-list>
                  </div>
                </ion-accordion>
              </ion-accordion-group>
            </ion-radio-group>

            <div ref="downloadAnchor"></div>

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
/* Reads as a link without shouting: same size as its neighbours in the meta
   line, accent-coloured, with the small external-link glyph carrying the
   "this leaves the app" meaning. */
.head .imdb-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--ion-color-primary);
  text-decoration: none;
}
.head .imdb-link ion-icon {
  font-size: 0.9em;
  opacity: 0.75;
}
.head .imdb-link:active {
  opacity: 0.6;
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
.season {
  color: var(--ion-color-primary, #3dc2ff);
  font-weight: 600;
}
.season-groups {
  margin: 0 8px 8px;
}
/* The header has to be readable at 360px too: it carries the episode list, which
   is the longest string on the screen. */
.season-sub {
  white-space: normal;
  overflow-wrap: anywhere;
  font-size: 0.78rem;
}
.season-sub.present {
  color: var(--app-status-finished);
}
.season-options {
  margin: 0;
}
.error {
  display: block;
  margin: 8px 4px;
}
.send-btn {
  margin-top: 16px;
}
</style>
