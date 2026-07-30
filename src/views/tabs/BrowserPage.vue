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
  IonTitle,
  IonToolbar,
  type InfiniteScrollCustomEvent,
  type RefresherCustomEvent,
  type ScrollDetail,
} from '@ionic/vue';
import {
  arrowDownOutline,
  arrowUpOutline,
  closeCircleOutline,
  closeOutline,
  funnelOutline,
  refreshOutline,
  settingsOutline,
  starOutline,
} from 'ionicons/icons';
import { posterSrc, type CatalogTitle } from '@/services/api';
import { splitYear } from '@/services/title-year';
import { useSourceCatalog } from '@/composables/useSourceCatalog';
import { SORTS, sortLabel } from '@/services/source-filters';
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
  filters,
  sort,
  order,
  query,
  searchActive,
  searchIneffective,
  loadStatus,
  runSearch,
  loadMore,
  setQuery,
  applyFilters,
  setSort,
  toggleOrder,
  clearFilters,
  removeFilter,
  loadPrefs,
  loadView,
  loadParameters,
  filterOptions,
  optionLabel,
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

const filterOpen = ref(false);
const titleOpen = ref(false);
const active = ref<CatalogTitle | null>(null);
const contentRef = ref<{ $el: { getScrollElement: () => Promise<HTMLElement> } } | null>(null);

// The sort dropdown (beside the search bar) shows the current order's short label.
const currentSortLabel = computed(() => sortLabel(sort.value));
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

// Re-check the source and re-apply the server-saved view (filters + sort), then
// reload — so opening/returning to Discover reflects the latest, including a
// change made on another device.
async function refreshView(): Promise<void> {
  await loadStatus();
  await loadView();
  if (!unavailable.value && !needsRefresh.value) {
    void loadParameters(); // refresh the filter facets from the source (non-blocking)
    await search(true);
  }
}

onMounted(async () => {
  await loadPrefs();
  await refreshView();
});
// Re-entering the Discover tab, and bringing the app to the foreground while on
// it, both re-sync the saved view from the server.
onIonViewWillEnter(refreshView);
function onForeground(): void {
  if (document.visibilityState === 'visible' && router.currentRoute.value.path.startsWith('/tabs/browser')) {
    void refreshView();
  }
}
onMounted(() => document.addEventListener('visibilitychange', onForeground));
onUnmounted(() => document.removeEventListener('visibilitychange', onForeground));

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
  await clearFilters();
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
function onScroll(e: CustomEvent<ScrollDetail>): void {
  showTop.value = e.detail.scrollTop > 500;
}
async function scrollTop(): Promise<void> {
  const el = await contentRef.value?.$el?.getScrollElement?.();
  el?.scrollTo({ top: 0, behavior: 'smooth' });
}

function openTitle(t: CatalogTitle): void {
  active.value = t;
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
          <ion-button
            v-if="hasFilters && !unavailable && !needsRefresh"
            aria-label="Clear filters"
            :disabled="loading"
            @click="onClear"
          >
            <ion-icon slot="icon-only" :icon="closeCircleOutline" />
          </ion-button>
          <ion-button
            v-if="!unavailable && !needsRefresh"
            :aria-label="'Filters'"
            :disabled="loading"
            @click="filterOpen = true"
          >
            <ion-icon slot="icon-only" :icon="funnelOutline" :color="hasFilters ? 'primary' : undefined" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
      <ion-toolbar v-if="!unavailable && !needsRefresh">
        <ion-searchbar
          :debounce="450"
          placeholder="Search for title"
          :value="query"
          @ionInput="onSearch"
        />
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
            <ion-select-option v-for="s in SORTS" :key="s.value" :value="s.value">
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
           change never looks like nothing happened. -->
      <ion-progress-bar
        v-if="loading && !unavailable && !needsRefresh"
        type="indeterminate"
        data-testid="search-loading"
      />
    </ion-header>

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
          <ion-chip v-if="filters.stream" @click="onRemoveFilter('stream')">
            Streamable<ion-icon :icon="closeOutline" />
          </ion-chip>
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
            </div>
            <ion-label class="meta">
              <h3>{{ displayTitle(t.title) }}</h3>
              <p>
                <span v-if="t.imdbScore">★ {{ t.imdbScore.toFixed(1) }}</span>
                <span v-if="yearOf(t.title)" class="year">{{ yearOf(t.title) }}</span>
                <span class="type">{{ t.type }}</span>
              </p>
            </ion-label>
          </button>
        </div>

        <!-- Load the next page well before the user hits the bottom so scrolling
             feels continuous (threshold is a large fraction of the viewport). -->
        <ion-infinite-scroll v-if="hasMore" threshold="60%" @ionInfinite="onInfinite">
          <ion-infinite-scroll-content />
        </ion-infinite-scroll>
      </template>

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
</style>
