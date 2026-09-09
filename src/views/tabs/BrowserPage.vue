<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import {
  onIonViewWillEnter,
  IonButton,
  IonButtons,
  IonChip,
  IonContent,
  IonFab,
  IonFabButton,
  IonHeader,
  IonIcon,
  IonInfiniteScroll,
  IonInfiniteScrollContent,
  IonLabel,
  IonNote,
  IonPage,
  IonProgressBar,
  IonRefresher,
  IonRefresherContent,
  IonSearchbar,
  IonSelect,
  IonSelectOption,
  IonSkeletonText,
  IonSpinner,
  IonToast,
  IonTitle,
  IonToolbar,
  type InfiniteScrollCustomEvent,
  type RefresherCustomEvent,
  type ScrollDetail,
} from '@ionic/vue';
import {
  arrowDownOutline,
  arrowUpOutline,
  alertCircleOutline,
  closeCircleOutline,
  closeOutline,
  funnelOutline,
  refreshOutline,
  settingsOutline,
  starOutline,
} from 'ionicons/icons';
import { posterSrc, type CatalogTitle } from '@/services/api';
import { logoForKind, monogram } from '@/services/source-logo';
import { splitYear } from '@/services/title-year';
import { displayYear } from '@/services/release-year';
import { genreLabels } from '@/services/genre-label';
import { useSourceCatalog } from '@/composables/useSourceCatalog';
import { sortLabel } from '@/services/source-filters';
import { useSession } from '@/composables/useSession';
import SourceFilterSheet from '@/components/SourceFilterSheet.vue';
import SourceTitleModal from '@/components/SourceTitleModal.vue';

const router = useRouter();
const { isAdmin } = useSession();
const {
  items,
  loading,
  needsRefresh,
  unavailable,
  errorMsg,
  hasMore,
  hasFilters,
  viewChanged,
  filters,
  sort,
  order,
  query,
  searchActive,
  searchIneffective,
  pendingOpen,
  pendingSearch,
  loadStatus,
  runSearch,
  restoreLastSession,
  viewMatchesResults,
  restored,
  cancellable,
  cancelSearch,
  loadMore,
  setQuery,
  applyFilters,
  setSort,
  toggleOrder,
  clearFilters,
  resetView,
  removeFilter,
  loadPrefs,
  loadView,
  loadParameters,
  filterOptions,
  optionLabel,
  parameters,
  sources,
  selectedSource,
  selectedSourceName,
  showSourcePicker,
  degraded,
  loadSources,
  setSource,
} = useSourceCatalog();

// Chip labels use the same (live-or-built-in) option lists as the filter sheet.
const typeChip = computed(() => optionLabel(filterOptions.value.types, filters.value.type));
const genreChip = computed(() => optionLabel(filterOptions.value.genres, filters.value.genre?.[0]));
const scoreChip = computed(() => optionLabel(filterOptions.value.scores, filters.value.score));
const languageChip = computed(() => optionLabel(filterOptions.value.languages, filters.value.language));
const countryChip = computed(() => optionLabel(filterOptions.value.countries, filters.value.country));
const yearChip = computed(() => {
  const { yearFrom: from, yearTo: to } = filters.value;
  if (from && to) return `${from}–${to}`;
  if (from) return `≥ ${from}`;
  if (to) return `≤ ${to}`;
  return '';
});

// The release year is embedded at the end of the provider's title; show it as a
// separate detail and strip it from the displayed name (the raw title is still
// what we send, so the created folder keeps the year).
function displayTitle(raw: string): string {
  return splitYear(raw).title;
}
function yearOf(raw: string): string {
  return splitYear(raw).year;
}

// The year the source published, falling back to the one at the end of the
// title — 30nama has no year field at all — and nothing when neither is
// believable. ZarFilm titles carry no trailing year, which is why they showed
// none at all before the source field started coming through (spec 1032).
function yearFor(t: CatalogTitle): string {
  return displayYear(t.year, t.title);
}

// Genres now have a line of their own (spec 1035), so more than one fits.
// Three rather than the sheet's four: a card is narrower, and beyond three the
// line clips anyway.
function genresFor(t: CatalogTitle): string[] {
  return genreLabels(t.genres, parameters.value?.genres, 3);
}

// Whether the middle line has anything at all to say. Without this a title with
// no type, year or rating would render an empty line and stand a pixel taller
// than its neighbours.
function factsFor(t: CatalogTitle): string[] {
  const out: string[] = [];
  if (!filters.value.type && t.type) out.push(t.type);
  if (yearFor(t)) out.push(yearFor(t));
  if (t.imdbScore) out.push(String(t.imdbScore));
  return out;
}

const filterOpen = ref(false);
const titleOpen = ref(false);
const active = ref<CatalogTitle | null>(null);
// The open title is read-only when it was reached from a task rather than from
// this list: that download already exists, so the sheet is the title's info page.
const titleInfoOnly = ref(false);
const contentRef = ref<{ $el: { getScrollElement: () => Promise<HTMLElement> } } | null>(null);

// Source labels appear on results only while more than one source is feeding the
// list. With a single source selected every result is from it, so the label would
// be noise on every card.
const showSourceLabels = computed(() => showSourcePicker.value && selectedSource.value === '');

// A card knows its sourceId; the bundled mark is keyed on the driver's KIND,
// which is stable where a display name is not. The configured sources are
// already loaded for the picker, so this is a lookup rather than a fetch.
const kindById = computed(() => {
  const out: Record<string, string> = {};
  for (const p of sources.value) out[String(p.id)] = p.kind;
  return out;
});
const sourceLogo = (t: CatalogTitle) => logoForKind(kindById.value[String(t.sourceId ?? '')]);

async function onSource(value: string): Promise<void> {
  const dropped = await setSource(value);
  await scrollTop();
  // FR-016: a filter the new selection can't honour is removed rather than left
  // applied and silently ignored — but the user has to be told, or the results
  // just change for no visible reason.
  if (dropped.length) {
    droppedNotice.value = `${dropped.join(', ')} ${dropped.length > 1 ? 'filters' : 'filter'} cleared — not available for this source.`;
  }
}
const droppedNotice = ref('');

// Plain-language summary of what dropped out, naming the source — an operator
// needs to know WHICH source to go and fix, and a user needs to know the list is
// incomplete rather than that the catalog shrank.
const degradedMessage = computed(() => {
  const names = degraded.value.map((d) => d.name).join(', ');
  if (!names) return '';
  const reason = degraded.value[0]?.reason;
  const why =
    reason === 'unsubscribed'
      ? 'needs an active subscription'
      : reason === 'needs_refresh'
        ? 'needs signing in again'
        : reason === 'filter_unsupported'
          ? "can't narrow by that"
          : "isn't responding";
  const verb = degraded.value.length > 1 ? 'are unavailable' : `${why}`;
  return degraded.value.length > 1
    ? `Some results are missing: ${names} ${verb}.`
    : `Some results are missing: ${names} ${verb}.`;
});

// The sort dropdown (beside the search bar) shows the current order's short label.
// Label the current ordering from the list actually on offer, so a source-only
// ordering reads correctly; sortLabel covers the built-in names while the live
// capabilities are still loading.
const currentSortLabel = computed(
  () => filterOptions.value.sorts.find((s) => s.value === sort.value)?.label ?? sortLabel(sort.value),
);
const sortOpen = ref(false);
async function onSort(value: string): Promise<void> {
  if (loading.value) return;
  sortOpen.value = false;
  await setSort(value);
  await fillViewport();
  await scrollTop();
}

// The direction toggle folded into the same sort control: its arrow flips to
// show whether the current sort runs descending (default) or ascending.
const orderIcon = computed(() => (order.value === 'asc' ? arrowUpOutline : arrowDownOutline));
const orderLabel = computed(() => (order.value === 'asc' ? 'Ascending' : 'Descending'));
async function onToggleOrder(): Promise<void> {
  if (loading.value) return;
  await toggleOrder();
  await fillViewport();
  await scrollTop();
}

// On a wide/tall desktop screen a page of results may not reach the bottom of
// the viewport, so ion-infinite-scroll never triggers and the list looks stuck.
// Keep loading pages until the content overflows the viewport (or nothing more).
async function fillViewport(): Promise<void> {
  for (let i = 0; i < 10; i += 1) {
    // Bail out as soon as the source is unavailable/needs-refresh/errored so we
    // don't keep firing page requests at a failing (rate-limited) provider.
    if (!hasMore.value || loading.value || needsRefresh.value || unavailable.value || errorMsg.value) return;
    await nextTick();
    const el = await contentRef.value?.$el?.getScrollElement?.();
    if (!el) return;
    if (el.scrollHeight > el.clientHeight + 48) return; // fills the screen
    await loadMore();
  }
}

async function search(reset: boolean): Promise<void> {
  await runSearch(reset);
  await fillViewport();
}

// A stable fingerprint of everything that decides which results are on screen.
// Key order is normalised because `filters` is rebuilt from the server's JSON on
// every loadView(), and an incidental reordering there must not read as a change.
function viewKey(): string {
  const f = filters.value as Record<string, unknown>;
  const entries = Object.keys(f)
    .sort()
    .map((k) => [k, f[k]]);
  return JSON.stringify([entries, sort.value, order.value, query.value, selectedSource.value]);
}

// Re-check the source and re-apply the server-saved view (filters + sort) — so
// opening/returning to Discover reflects the latest, including a change made on
// another device.
//
// Returning to the tab must NOT throw the loaded results away. Re-running the
// search drops every page the user had scrolled through and rebuilds the grid
// from page 1, which lands them back near the top having lost their place —
// exactly what happens after diving into a title, sending it, and coming back
// via Tasks. So reload only when it would actually show something different:
// the saved view changed under us, or there is nothing on screen yet. The cost
// is that results can be a little stale; pull-to-refresh reloads on demand.
async function refreshView(): Promise<void> {
  await loadStatus();
  // The source list feeds the selector and decides whether it is shown at all.
  void loadSources();
  await loadView();
  if (unavailable.value || needsRefresh.value) return;
  void loadParameters(); // refresh the filter facets from the source (non-blocking)
  // Keep what is on screen when it still matches what the controls say. Asked of
  // the results themselves rather than of a before/after comparison (spec 1041),
  // so a session RESTORED from the last launch is kept for the same reason a
  // freshly searched one is — there is only one rule, and it is about whether the
  // screen is honest.
  if (items.value.length > 0 && viewMatchesResults()) return;
  await search(true);
  // The view genuinely changed, so this is a different result set — the old
  // scroll offset would drop the user into the middle of it.
  await scrollTop();
}

// "Open in Discover" handoff from the Tasks tab (spec 1016): open the exact
// title's modal, or run a search when only a title string was handed over.
async function consumePendingOpen(): Promise<void> {
  if (pendingOpen.value) {
    const t = pendingOpen.value;
    pendingOpen.value = null;
    pendingSearch.value = '';
    // A pending title only ever comes from a task's "Open in Discover", so it
    // opens read-only — the user wants the title's page, not another download.
    openTitle(t, true);
  } else if (pendingSearch.value) {
    const q = pendingSearch.value;
    pendingSearch.value = '';
    await setQuery(q);
  }
}

/**
 * Opening the app (spec 1041).
 *
 * Deliberately NOT refreshView(). Discover used to search on every launch —
 * page one, plus however many more it took to fill the grid — against a source
 * that is shared and rate-limited, for a reader who may not have come here at
 * all. Now the last session comes back and nothing is asked.
 *
 * Only when there is nothing to come back to does it search, using the filter
 * and sort as they are set, default or not.
 */
/**
 * Whether the first load has happened.
 *
 * Ionic fires ionViewWillEnter BEFORE onMounted — measured, not assumed, and the
 * opposite of what an earlier version of this guard supposed. So on a cold start
 * the enter hook ran first, found nothing on screen and searched, and the mount
 * then restored the session into a grid that had already been fetched: the
 * restore worked and the request was made anyway.
 *
 * The mount owns the first pass. The enter hook does nothing until it has run,
 * and takes over from the second entry onwards (spec 1041).
 */
let initialised = false;

onMounted(() => firstLoad());

async function firstLoad(): Promise<void> {
  await loadPrefs();
  await loadStatus();
  // The source list has to be in hand BEFORE restoring: a remembered view whose
  // source has since gone is discarded rather than shown under another's name.
  await loadSources();
  await loadView();
  if (unavailable.value || needsRefresh.value) {
    await consumePendingOpen();
    initialised = true;
    return;
  }
  void loadParameters();
  if (!(await restoreLastSession())) {
    await search(true);
  }
  await consumePendingOpen();
  initialised = true;
}
// Re-entering the Discover tab, and bringing the app to the foreground while on
// it, both re-sync the saved view from the server; entering also honours any
// pending "Open in Discover" request from the Tasks tab.
onIonViewWillEnter(async () => {
  // The first entry is the mount's to handle; this hook runs before it.
  if (!initialised) return;
  await refreshView();
  await consumePendingOpen();
});
function onForeground(): void {
  if (document.visibilityState === 'visible' && router.currentRoute.value.path.startsWith('/tabs/browser')) {
    void refreshView();
  }
}
onMounted(() => document.addEventListener('visibilitychange', onForeground));
onUnmounted(() => document.removeEventListener('visibilitychange', onForeground));

/**
 * Take the search back.
 *
 * The grid keeps the scroll position it had — the results being restored are the
 * ones that were on screen a moment ago, so moving the reader would be the
 * jarring part rather than the fix.
 */
function onCancelSearch(): void {
  cancelSearch();
}

async function onSearch(e: CustomEvent): Promise<void> {
  await setQuery(((e.detail as { value?: string }).value ?? '').trim());
  await fillViewport();
  // A new search replaces the list, so a previously scrolled-down view would
  // otherwise land the user in the middle of the fresh results — jump back up.
  await scrollTop();
}

async function onApply(f: typeof filters.value): Promise<void> {
  await applyFilters(f);
  await fillViewport();
  await scrollTop();
}

async function onClear(): Promise<void> {
  if (loading.value) return;
  // The whole view, not only the filters: the button appears for a changed sort
  // as well, so it has to undo that too.
  await resetView();
  await fillViewport();
  await scrollTop();
}

async function onRemoveFilter(key: Parameters<typeof removeFilter>[0]): Promise<void> {
  if (loading.value) return; // don't change the query while a search is in flight
  await removeFilter(key);
  await fillViewport();
  await scrollTop();
}

async function onInfinite(e: InfiniteScrollCustomEvent): Promise<void> {
  await loadMore();
  await e.target.complete();
}

// Pull-to-refresh: re-check the source and reload the current view.
async function onRefresh(e: RefresherCustomEvent): Promise<void> {
  await loadStatus();
  if (!unavailable.value && !needsRefresh.value) await search(true);
  await e.target.complete();
}

// Show a jump-to-top button once the list is scrolled down a screenful or so.
const showTop = ref(false);
/**
 * True while a restored session is on screen and the reader has not moved yet.
 *
 * One genuine scroll is enough to mean "show me more", and any search clears the
 * restored flag anyway.
 */
const scrolledSinceRestore = ref(false);
const holdInfinite = computed(() => restored.value && !scrolledSinceRestore.value);

function onScroll(e: CustomEvent<ScrollDetail>): void {
  showTop.value = e.detail.scrollTop > 500;
  if (e.detail.scrollTop > 0) scrolledSinceRestore.value = true;
}
async function scrollTop(): Promise<void> {
  const el = await contentRef.value?.$el?.getScrollElement?.();
  el?.scrollTo({ top: 0, behavior: 'smooth' });
}

function openTitle(t: CatalogTitle, infoOnly = false): void {
  active.value = t;
  titleInfoOnly.value = infoOnly;
  titleOpen.value = true;
}

// Some provider cover URLs are dead (present but 404). On the first load error we
// retry the title's reliable fallback poster (the provider's sized/placeholder
// image); only if that also fails do we drop to the letter tile.
const failedPosters = ref<Set<string>>(new Set());
const fellBackPosters = ref<Set<string>>(new Set());
function posterFor(t: CatalogTitle): string {
  if (fellBackPosters.value.has(t.id)) return posterSrc(t.posterFallbackUrl ?? '');
  return posterSrc(t.posterUrl);
}
function onPosterError(t: CatalogTitle): void {
  if (!fellBackPosters.value.has(t.id) && t.posterFallbackUrl && t.posterFallbackUrl !== t.posterUrl) {
    fellBackPosters.value = new Set(fellBackPosters.value).add(t.id);
    return;
  }
  if (!failedPosters.value.has(t.id)) {
    failedPosters.value = new Set(failedPosters.value).add(t.id);
  }
}

async function retry(): Promise<void> {
  await loadStatus();
  if (!unavailable.value && !needsRefresh.value) await search(true);
}

function goSettings(): void {
  router.push('/tabs/settings');
}
</script>

<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>Discover</ion-title>
        <ion-buttons slot="end">
          <!-- Shown whenever the view differs from default in ANY way — a
               filter OR a changed sort — so there is always a one-tap way back.
               Green so it reads as the counterpart to the filter funnel beside
               it rather than as a warning. -->
          <ion-button
            v-if="viewChanged && !unavailable && !needsRefresh"
            aria-label="Reset filters and sort"
            data-testid="discover-reset"
            :disabled="loading"
            @click="onClear"
          >
            <ion-icon slot="icon-only" :icon="closeCircleOutline" color="success" />
          </ion-button>
          <ion-button
            v-if="!unavailable && !needsRefresh"
            :aria-label="'Filters'"
            :disabled="loading"
            data-testid="filter-open"
            @click="filterOpen = true"
          >
            <ion-icon slot="icon-only" :icon="funnelOutline" :color="viewChanged ? 'success' : undefined" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
      <ion-toolbar v-if="!unavailable && !needsRefresh">
        <ion-searchbar
          :debounce="450"
          placeholder="Search"
          :value="query"
          data-testid="discover-search"
          @ionInput="onSearch"
        />
        <!-- Source selector, shown only once more than one source is configured:
             with a single source it would be a control with nothing to select.
             "All sources" is the first entry and the default. -->
        <div v-if="showSourcePicker" slot="end" class="source-control">
          <ion-select
            class="source-select"
            :value="selectedSource"
            :disabled="loading"
            interface="popover"
            aria-label="Source"
            :selected-text="selectedSourceName"
            @ionChange="(e) => onSource(String(e.detail.value))"
          >
            <ion-select-option value="">All sources</ion-select-option>
            <ion-select-option v-for="s in sources" :key="s.id" :value="String(s.id)">
              {{ s.displayName }}
            </ion-select-option>
          </ion-select>
        </div>
        <!-- Sort control beside the search bar: the field popover plus a direction
             toggle whose arrow shows ascending vs descending — one control, no
             second dropdown. Disabled while a search is running (the in-flight
             query can't be changed out from under itself) AND while a text query
             is active, because the source can't sort text-search results — see
             the hint below (spec 2002). -->
        <div slot="end" class="sort-control" :class="{ ineffective: searchActive }">
          <ion-select
            class="sort-select"
            :value="sort"
            :disabled="loading || searchActive"
            interface="popover"
            aria-label="Sort by"
            :selected-text="currentSortLabel"
            @ionChange="(e) => onSort(String(e.detail.value))"
          >
            <ion-select-option v-for="s in filterOptions.sorts" :key="s.value" :value="s.value">
              {{ s.label }}
            </ion-select-option>
          </ion-select>
          <ion-button
            class="order-toggle"
            fill="clear"
            size="small"
            :disabled="loading || searchActive"
            :aria-label="`Sort direction: ${orderLabel}`"
            :title="orderLabel"
            @click="onToggleOrder"
          >
            <ion-icon slot="icon-only" :icon="orderIcon" />
          </ion-button>
        </div>
      </ion-toolbar>
      <!-- A slim bar signals a search is running even when results are already on
           screen (the provider can take a few seconds), so a slow filter/sort
           change never looks like nothing happened.
           It keeps its 4px row at all times rather than being added and removed:
           mounting it grows the header, which pushes the whole list down and then
           lets it snap back when the load finishes — a small but very visible
           jump every time infinite scroll pulls a page. Hiding it instead leaves
           the header exactly as tall whether or not a search is running. -->
      <ion-progress-bar
        v-if="!unavailable && !needsRefresh"
        type="indeterminate"
        class="search-bar"
        :class="{ idle: !loading }"
        data-testid="search-loading"
      />
    </ion-header>

    <!-- Calling a search off (spec 1041).
         OUTSIDE the header and positioned over the content, so it costs no
         layout: the header keeps exactly the height it had, and nothing on
         screen moves as this appears or goes (FR-011). The same reason the
         progress bar above holds its row rather than being mounted and
         unmounted. -->
    <transition name="cancel-pill">
      <button
        v-if="cancellable"
        type="button"
        class="cancel-pill"
        data-testid="search-cancel"
        @click="onCancelSearch"
      >
        <ion-spinner name="crescent" class="pill-spinner" />
        <span class="pill-text">Searching</span>
        <ion-icon :icon="closeCircleOutline" class="pill-x" />
        <span class="pill-action">Cancel</span>
      </button>
    </transition>

    <ion-content ref="contentRef" :fullscreen="true" :scroll-events="true" @ionScroll="onScroll">
      <ion-refresher slot="fixed" @ionRefresh="onRefresh">
        <ion-refresher-content />
      </ion-refresher>

      <!-- Unavailable: no provider configured (or legacy mode). -->
      <div v-if="unavailable" class="state">
        <ion-icon :icon="starOutline" class="state-icon" />
        <p>No download source is set up yet.</p>
        <template v-if="isAdmin">
          <ion-note>Configure one in Settings to browse and send downloads.</ion-note>
          <ion-button fill="outline" class="cta" @click="goSettings">
            <ion-icon slot="start" :icon="settingsOutline" />
            Set up a source
          </ion-button>
        </template>
        <ion-note v-else>Ask an admin to configure a download source.</ion-note>
      </div>

      <!-- Needs refresh: the stored session expired. -->
      <div v-else-if="needsRefresh" class="state">
        <ion-icon :icon="refreshOutline" class="state-icon" />
        <p>The download source needs refreshing.</p>
        <template v-if="isAdmin">
          <ion-note>Re-paste the source session in Settings to bring it back.</ion-note>
          <ion-button fill="outline" class="cta" @click="goSettings">
            <ion-icon slot="start" :icon="settingsOutline" />
            Refresh the source
          </ion-button>
        </template>
        <ion-note v-else>Ask an admin to refresh the download source.</ion-note>
      </div>

      <template v-else>
        <!-- While searching, the source only ranks results by relevance and honors
             the type filter. The hint lives here (not pinned to the header) so it
             scrolls away with the chips. The "clear the search" nudge only appears
             when the user actually has a selection that's being ignored (spec 1014). -->
        <p v-if="searchActive" class="search-hint" data-testid="search-hint">
          Search results are ranked by relevance. Only the type filter narrows them.
          <template v-if="searchIneffective">
            Clear the search to sort or use the other filters.
          </template>
        </p>
        <!-- Active filters as removable chips: tap one to drop just that filter
             without reopening the sheet. While searching, everything except the type
             chip is struck through to show it isn't applied right now. -->
        <div
          v-if="hasFilters"
          class="active-filters"
          :class="{ busy: loading, ineffective: searchActive }"
        >
          <ion-chip v-if="filters.type" class="cap" @click="onRemoveFilter('type')">
            {{ typeChip }}<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.genre?.length" @click="onRemoveFilter('genre')">
            {{ genreChip }}<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.quality" @click="onRemoveFilter('quality')">
            {{ filters.quality }}<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.score" @click="onRemoveFilter('score')">
            ★ {{ scoreChip }}<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.language" @click="onRemoveFilter('language')">
            {{ languageChip }}<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.country" @click="onRemoveFilter('country')">
            {{ countryChip }}<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.channel" @click="onRemoveFilter('channel')">
            {{ filters.channel }}<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.encoder" @click="onRemoveFilter('encoder')">
            {{ filters.encoder }}<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.age" @click="onRemoveFilter('age')">
            {{ filters.age }}<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="yearChip" @click="onRemoveFilter('yearFrom'); onRemoveFilter('yearTo')">
            {{ yearChip }}<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.cast" @click="onRemoveFilter('cast')">
            Cast: {{ filters.cast }}<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.director" @click="onRemoveFilter('director')">
            Director: {{ filters.director }}<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.creator" @click="onRemoveFilter('creator')">
            Creator: {{ filters.creator }}<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.x265" @click="onRemoveFilter('x265')">
            x265<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.threeD" @click="onRemoveFilter('threeD')">
            3D<ion-icon :icon="closeOutline" />
          </ion-chip>
        </div>

        <!-- A source that couldn't answer this query. Deliberately outside the
             results branch below and never blocking: the healthy sources' results
             render underneath it, which is the whole point of degrading rather
             than failing. -->
        <div v-if="degraded.length" class="degraded" role="status">
          <ion-icon :icon="alertCircleOutline" aria-hidden="true" />
          <span>{{ degradedMessage }}</span>
        </div>

        <div v-if="errorMsg" class="state">
          <ion-note color="danger">{{ errorMsg }}</ion-note>
          <ion-button fill="outline" class="cta" @click="retry">Try again</ion-button>
        </div>

        <!-- Skeleton poster placeholders while the first results load, instead of
             a lone spinner on a black screen. -->
        <div v-else-if="loading && items.length === 0" class="grid" aria-hidden="true">
          <div v-for="n in 12" :key="n" class="card">
            <div class="poster">
              <ion-skeleton-text :animated="true" class="sk-poster" />
            </div>
            <ion-label class="meta">
              <ion-skeleton-text :animated="true" class="sk-line" />
              <ion-skeleton-text :animated="true" class="sk-line short" />
            </ion-label>
          </div>
        </div>

        <div v-else-if="items.length === 0" class="state">
          <ion-note>No results. Try a different search or filters.</ion-note>
        </div>

        <div v-else class="grid">
          <button
            v-for="t in items"
            :key="t.id"
            type="button"
            class="card"
            data-testid="catalog-card"
            @click="openTitle(t)"
          >
            <div class="poster">
              <img
                v-if="(t.posterUrl || t.posterFallbackUrl) && !failedPosters.has(t.id)"
                :src="posterFor(t)"
                :alt="t.title"
                loading="lazy"
                @error="onPosterError(t)"
              />
              <div v-else class="poster-fallback">{{ t.title.charAt(0) }}</div>
              <span v-if="t.comingSoon" class="badge">Soon</span>
              <!-- Already on the NAS (spec 0008), as a folded corner ribbon.
                   A title you own is one you want to SKIP, so the mark has to be
                   readable in peripheral vision across a whole grid without
                   competing with the artwork — a corner is dead space, and a
                   ribbon there never covers the poster. Anchored top-RIGHT so it
                   cannot collide with the "Soon" badge on the left; a title can
                   legitimately be both. The word is spelled out rather than
                   left as a tick, which reads as "selected" or "verified" just
                   as easily. "OWNED" rather than "DOWNLOADED" because it may have
                   arrived by any route — the mark means the VIDEO is on the NAS,
                   not merely that a folder of that name exists. A folder holding
                   only artwork and metadata is not owned (FR-001a). -->
              <span
                v-if="t.ownership === 'owned'"
                class="ribbon"
                aria-label="Already in your library"
              >
                <span class="ribbon-band">OWNED</span>
              </span>
              <!-- Still arriving. Download Station writes the video into the
                   destination as it goes, so this title HAS a video file and
                   would otherwise read as owned — but the advice differs: wait
                   for it rather than skip it (FR-001b). Anything not yet checked
                   carries no mark at all (FR-010c). -->
              <span
                v-else-if="t.ownership === 'downloading'"
                class="ribbon"
                aria-label="Downloading now"
              >
                <span class="ribbon-band getting">DOWNLOADING</span>
              </span>
              <!-- Only in combined mode: a title carried by two sources appears
                   twice, and without a mark that reads as a duplicate rather
                   than as two sources offering it. Redundant — and so omitted —
                   when a single source is selected. Bottom-right keeps it clear
                   of both corner marks above. -->
              <span v-if="showSourceLabels && t.sourceName" class="src-mark" :title="t.sourceName">
                <img
                  v-if="sourceLogo(t)"
                  :src="sourceLogo(t)"
                  :alt="t.sourceName"
                  loading="lazy"
                />
                <span v-else class="src-mono">{{ monogram(t.sourceName) }}</span>
              </span>
            </div>
            <ion-label class="meta">
              <h3>{{ displayTitle(t.title) }}</h3>
              <!-- Three lines matching the detail sheet's header, in its order
                   (spec 1035), so tapping a card does not rearrange the facts
                   the user was just reading. Each <p> is omitted entirely when
                   it has nothing to say, so a title missing a rating or a genre
                   does not leave a blank line and knock the grid out of line. -->
              <p v-if="factsFor(t).length" class="facts">
                <span v-if="t.imdbScore">★ {{ t.imdbScore.toFixed(1) }}</span>
                <span v-if="!filters.type" class="type">{{ t.type }}</span>
                <span v-if="yearFor(t)" class="year">{{ yearFor(t) }}</span>
              </p>
              <p v-if="genresFor(t).length" class="genres">
                {{ genresFor(t).join(' · ') }}
              </p>
            </ion-label>
          </button>
        </div>

        <!-- Load the next pages well before the user hits the bottom so scrolling
             feels continuous: the trigger fires a full viewport early, and each
             one pulls two pages (see PAGES_PER_LOAD), so a fast flick doesn't
             outrun the grid. -->
        <!-- Held back on a session restored from the last launch until the
             reader actually scrolls (spec 1041).
             `threshold="100%"` means this fires the moment the grid is shorter
             than the viewport — which, after restoring, is a page request on
             open by another name. The whole point is that opening the app asks
             the source nothing, so it waits for a scroll to say otherwise. -->
        <ion-infinite-scroll
          v-if="hasMore && !holdInfinite"
          threshold="100%"
          @ionInfinite="onInfinite"
        >
          <ion-infinite-scroll-content />
        </ion-infinite-scroll>
      </template>

      <ion-toast
        :is-open="droppedNotice !== ''"
        :message="droppedNotice"
        :duration="4000"
        @didDismiss="droppedNotice = ''"
      />

      <!-- Appears only once scrolled down; taps back to the top. -->
      <ion-fab v-show="showTop" slot="fixed" vertical="bottom" horizontal="end">
        <ion-fab-button class="app-fab" aria-label="Jump to top" @click="scrollTop">
          <ion-icon :icon="arrowUpOutline" />
        </ion-fab-button>
      </ion-fab>
    </ion-content>

    <SourceFilterSheet
      :is-open="filterOpen"
      :filters="filters"
      @dismiss="filterOpen = false"
      @apply="onApply"
    />
    <SourceTitleModal
      v-if="active"
      :is-open="titleOpen"
      :title="active"
      :info-only="titleInfoOnly"
      @dismiss="titleOpen = false"
      @needs-refresh="retry"
    />
  </ion-page>
</template>

<style scoped>
.state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding-top: 22vh;
  text-align: center;
  color: var(--app-text-dim);
}
.state-icon {
  font-size: 44px;
  color: var(--app-text-dim);
}
.cta {
  margin-top: 8px;
}
.sort-control {
  display: flex;
  align-items: center;
}
/* The source selector sits beside the sort control and matches it, so the two
   read as one row of view controls rather than a bolted-on extra. */
.source-control {
  display: flex;
  align-items: center;
}
.source-select {
  max-width: 9.5rem;
  font-size: 0.85rem;
  --padding-start: 0.5rem;
  --padding-end: 0.25rem;
}
/* Which source a result came from, in combined mode only. Deliberately quiet:
   it disambiguates a repeated title, it is not a headline. */
/* A source dropped out. Sits above the results it could not contribute to and
   never replaces them. */
/* Loud enough to notice. It used to be grey note text the same size and colour
   as everything around it, so "this source isn't responding" — the one line that
   explains why half the catalog is missing — read as decoration and was missed. */
.degraded {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin: 0.5rem 1rem;
  padding: 0.55rem 0.7rem;
  border: 1px solid var(--ion-color-warning, #ffc409);
  border-radius: 8px;
  background: rgba(255, 196, 9, 0.12);
  color: var(--ion-color-warning, #ffc409);
  font-size: 0.82rem;
  line-height: 1.35;
}
.degraded ion-icon {
  flex: 0 0 auto;
  font-size: 1.1rem;
}
/* While searching, sort can't be applied — dim the control and strike its label so
   it reads as inactive (it is also disabled). */
.sort-control.ineffective {
  opacity: 0.45;
}
.sort-control.ineffective .sort-select::part(text) {
  text-decoration: line-through;
}
/* The search hint now scrolls with the results (it sits above the chips), so it is a
   plain block in the content rather than a pinned toolbar. */
.search-hint {
  display: block;
  margin: 0;
  padding: 10px 16px 4px;
  font-size: 0.8rem;
  line-height: 1.35;
  color: var(--app-text-dim);
}
/* Strike through every active-filter chip except the type chip (.cap) while a
   search is active — those filters aren't applied to text-search results. */
.active-filters.ineffective ion-chip:not(.cap) {
  text-decoration: line-through;
  opacity: 0.5;
}
/* Idle keeps the bar's box (so the header height never changes) while hiding it
   from sight and from the accessibility tree — visibility, not opacity, so a
   hidden bar doesn't read as visible to tests or screen readers. */
.search-bar.idle {
  visibility: hidden;
}
.sort-select {
  max-width: 40vw;
  font-size: 0.85rem;
  --padding-end: 4px;
}
.order-toggle {
  --padding-start: 4px;
  --padding-end: 6px;
  margin: 0;
  height: 32px;
}
.active-filters {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 8px 12px 0;
}
/* While a search runs, the chips are dimmed and non-interactive so a slow query
   can't be changed mid-flight (the handlers also guard against it). */
.active-filters.busy {
  opacity: 0.55;
  pointer-events: none;
}
.active-filters .cap {
  text-transform: capitalize;
}
.grid {
  display: grid;
  /* Phones: compact tiles so a couple fit per row. Wider screens (tablets,
     desktop) get a larger minimum so posters scale up instead of packing a dozen
     tiny tiles across a wide window. */
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 12px;
  padding: 12px;
}
@media (min-width: 700px) {
  .grid {
    grid-template-columns: repeat(auto-fill, minmax(185px, 1fr));
    gap: 16px;
    padding: 16px;
  }
}
@media (min-width: 1100px) {
  .grid {
    grid-template-columns: repeat(auto-fill, minmax(215px, 1fr));
  }
}
@media (min-width: 1600px) {
  .grid {
    grid-template-columns: repeat(auto-fill, minmax(245px, 1fr));
    gap: 20px;
  }
}
.card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 0;
  border: none;
  background: transparent;
  text-align: left;
  cursor: pointer;
  /* Reset the native button look: iOS renders button text in link-blue and a
     system font — force the app's own text colour and font instead. */
  -webkit-appearance: none;
  appearance: none;
  font: inherit;
  color: var(--app-text);
}
.poster {
  position: relative;
  aspect-ratio: 2 / 3;
  border-radius: 10px;
  overflow: hidden;
  background: var(--ion-color-step-100, #1c1c1e);
}
.poster img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.poster-fallback {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 2rem;
  color: var(--app-text-dim);
}
.sk-poster {
  width: 100%;
  height: 100%;
  margin: 0;
}
.sk-line {
  height: 12px;
  border-radius: 4px;
  margin-top: 6px;
}
.sk-line.short {
  width: 45%;
}
.badge {
  position: absolute;
  top: 6px;
  left: 6px;
  padding: 2px 6px;
  font-size: 0.7rem;
  border-radius: 6px;
  background: rgba(0, 0, 0, 0.65);
  color: #fff;
}
/* The "you already have this" ribbon: a band rotated across the top-right
   corner, clipped by a square so it reads as folded over the poster's edge. The
   corner is dead space, so the mark costs no artwork — which is what lets it be
   bold enough to catch the eye in a grid without shouting. */
.ribbon {
  position: absolute;
  top: 0;
  right: 0;
  width: 68px;
  height: 68px;
  overflow: hidden;
  pointer-events: none;
  /* Match the poster's rounded corner so the ribbon is clipped by the same
     curve rather than poking past it. */
  border-top-right-radius: inherit;
}
.ribbon-band.getting {
  /* A distinct hue AND a distinct word: colour alone must not carry the
     difference (FR-012). Blue-700 takes white at roughly 8:1, and the longer
     word needs the smaller size to sit inside the band. */
  background: #1d4ed8;
  font-size: 0.5rem;
  letter-spacing: 0.02em;
}
.ribbon-band {
  position: absolute;
  top: 14px;
  right: -26px;
  width: 100px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 2px 0;
  transform: rotate(45deg);
  /* A deeper green than --app-status-finished on purpose. That green is right
     for a small icon, but white text on it is 2.3:1 — well under the 4.5:1 a
     label this size needs. This shade takes white at 5.0:1. */
  background: #15803d;
  /* Posters are arbitrary artwork, so the ribbon has to separate itself from
     whatever sits under it. A light edge plus a shadow does that against any
     image — changing the HUE would not: a brighter fill still disappears on a
     poster that happens to share it. The edge is slightly translucent so it
     reads as a rim on both light and dark posters rather than a hard sticker. */
  border: 1.5px solid rgba(255, 255, 255, 0.92);
  box-shadow:
    0 1px 5px rgba(0, 0, 0, 0.55),
    0 0 0 1px rgba(0, 0, 0, 0.28);
  color: #fff;
  /* Keeps the letters legible where the band crosses a bright patch. */
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.5);
  font-size: 0.68rem;
  font-weight: 800;
  letter-spacing: 0.06em;
  line-height: 1.3;
}

/* The source mark. Bottom-right, clear of both corner marks above, on the same
   dark scrim the badges use so it stays legible over any artwork. */
.src-mark {
  position: absolute;
  /* Bottom-LEFT: the poster's own logos and billing tend to sit bottom-right,
     and the top corners are taken by the "Soon" and "OWNED" marks. */
  left: 5px;
  bottom: 5px;
  display: flex;
  align-items: center;
  padding: 3px 5px;
  border-radius: 5px;
  background: rgba(0, 0, 0, 0.62);
  pointer-events: none;
}
.src-mark img {
  display: block;
  height: 15px;
  width: auto;
  /* Both marks are light-on-transparent, so they read on the scrim as they are;
     the slight fade stops them competing with the poster. */
  opacity: 0.92;
}
.src-mono {
  font-size: 0.7rem;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: #fff;
  opacity: 0.92;
}
.meta h3 {
  font-size: 0.9rem;
  font-weight: 600;
  margin: 0;
  color: var(--app-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.meta p {
  margin: 2px 0 0;
  font-size: 0.78rem;
  color: var(--app-text-dim);
  display: flex;
  gap: 8px;
}
.meta .type {
  text-transform: capitalize;
}
.meta .year {
  font-variant-numeric: tabular-nums;
}
/* Both fact lines clip rather than wrap. A caption that grew a fourth line
   would push its neighbours out of alignment, and the grid reads as a grid only
   while every cell is the same height (FR-005, FR-006). */
.meta .facts,
.meta .genres {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
/* The genre line is one string rather than flex items, so it ellipsises as a
   sentence would — "Crime · Drama · Thr…" — instead of dropping a whole genre. */
.meta .genres {
  display: block;
}
/* The cancel pill (spec 1041).
   Fixed rather than in flow, so it costs no layout at all: nothing on screen
   moves as it appears or goes, which is the same reason the progress bar above
   it holds its 4px row instead of being mounted and unmounted.
   Centred under the header and clear of it, so it reads as belonging to the
   search that is running rather than to any one control. */
.cancel-pill {
  position: fixed;
  top: calc(var(--app-header-h, 108px) + 10px);
  left: 50%;
  transform: translateX(-50%);
  z-index: 20;
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 7px 14px 7px 11px;
  border: none;
  border-radius: 999px;
  font: inherit;
  font-size: 0.82rem;
  color: var(--ion-text-color);
  /* Sits over the grid, so it needs to be legible against a poster as well as
     against the page. A blur plus a border does that in both themes without
     picking a colour that fights either one. */
  background: var(--ion-color-step-100, rgba(28, 28, 30, 0.86));
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border: 1px solid rgba(var(--ion-text-color-rgb, 255, 255, 255), 0.12);
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.28);
  cursor: pointer;
}
.cancel-pill:active {
  transform: translateX(-50%) scale(0.97);
}
.pill-spinner {
  width: 15px;
  height: 15px;
  color: var(--ion-color-primary);
}
.pill-text {
  color: var(--app-text-dim);
}
.pill-x {
  font-size: 15px;
  color: var(--ion-color-primary);
  margin-inline-start: 2px;
}
.pill-action {
  font-weight: 600;
  color: var(--ion-color-primary);
}
/* Fades and lifts rather than appearing — a control that offers to undo
   something should not itself feel abrupt. */
.cancel-pill-enter-active,
.cancel-pill-leave-active {
  transition: opacity 160ms ease, transform 160ms ease;
}
.cancel-pill-enter-from,
.cancel-pill-leave-to {
  opacity: 0;
  transform: translateX(-50%) translateY(-6px);
}
@media (prefers-reduced-motion: reduce) {
  .cancel-pill-enter-active,
  .cancel-pill-leave-active {
    transition: none;
  }
}
</style>
