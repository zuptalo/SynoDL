<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import {
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
  IonRefresher,
  IonRefresherContent,
  IonSearchbar,
  IonSkeletonText,
  IonTitle,
  IonToolbar,
  type InfiniteScrollCustomEvent,
  type RefresherCustomEvent,
  type ScrollDetail,
} from '@ionic/vue';
import {
  arrowUpOutline,
  closeCircleOutline,
  closeOutline,
  funnelOutline,
  refreshOutline,
  settingsOutline,
  starOutline,
} from 'ionicons/icons';
import { posterSrc, type CatalogTitle } from '@/services/api';
import { useSourceCatalog } from '@/composables/useSourceCatalog';
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
  query,
  loadStatus,
  runSearch,
  loadMore,
  setQuery,
  applyFilters,
  clearFilters,
  removeFilter,
  loadPrefs,
} = useSourceCatalog();

const filterOpen = ref(false);
const titleOpen = ref(false);
const active = ref<CatalogTitle | null>(null);
const contentRef = ref<{ $el: { getScrollElement: () => Promise<HTMLElement> } } | null>(null);

const sortLabel = computed(
  () => ({ date: 'Recently added', favorite: 'Popular' })[sort.value] ?? 'Release year',
);

// On a wide/tall desktop screen a page of results may not reach the bottom of
// the viewport, so ion-infinite-scroll never triggers and the list looks stuck.
// Keep loading pages until the content overflows the viewport (or nothing more).
async function fillViewport(): Promise<void> {
  for (let i = 0; i < 10; i += 1) {
    if (!hasMore.value || loading.value) return;
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

onMounted(async () => {
  await loadStatus();
  await loadPrefs();
  if (!unavailable.value && !needsRefresh.value) await search(true);
});

async function onSearch(e: CustomEvent): Promise<void> {
  await setQuery(((e.detail as { value?: string }).value ?? '').trim());
  await fillViewport();
}

async function onApply(f: typeof filters.value, s: string): Promise<void> {
  await applyFilters(f, s);
  await fillViewport();
}

async function onClear(): Promise<void> {
  await clearFilters();
  await fillViewport();
}

async function onRemoveFilter(key: Parameters<typeof removeFilter>[0]): Promise<void> {
  await removeFilter(key);
  await fillViewport();
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

// Some provider cover URLs are dead; swap those to the letter fallback instead of
// showing a broken-image icon or the raw alt text.
const failedPosters = ref<Set<string>>(new Set());
function onPosterError(id: string): void {
  if (!failedPosters.value.has(id)) {
    failedPosters.value = new Set(failedPosters.value).add(id);
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
            @click="onClear"
          >
            <ion-icon slot="icon-only" :icon="closeCircleOutline" />
          </ion-button>
          <ion-button
            v-if="!unavailable && !needsRefresh"
            :aria-label="'Filters'"
            @click="filterOpen = true"
          >
            <ion-icon slot="icon-only" :icon="funnelOutline" :color="hasFilters ? 'primary' : undefined" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
      <ion-toolbar v-if="!unavailable && !needsRefresh">
        <ion-searchbar
          :debounce="400"
          placeholder="Search movies, series, anime"
          :value="query"
          @ionInput="onSearch"
        />
      </ion-toolbar>
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
        <!-- Active filters as removable chips: tap one to drop just that filter
             without reopening the sheet. -->
        <div v-if="hasFilters" class="active-filters">
          <ion-chip v-if="sort !== 'year'" @click="onRemoveFilter('sort')">
            {{ sortLabel }}<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.type" class="cap" @click="onRemoveFilter('type')">
            {{ filters.type }}<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.genre?.length" @click="onRemoveFilter('genre')">
            Genre<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.quality" @click="onRemoveFilter('quality')">
            {{ filters.quality }}<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.score" @click="onRemoveFilter('score')">
            {{ filters.score }}+<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.language" @click="onRemoveFilter('language')">
            {{ filters.language }}<ion-icon :icon="closeOutline" />
          </ion-chip>
          <ion-chip v-if="filters.country" @click="onRemoveFilter('country')">
            {{ filters.country }}<ion-icon :icon="closeOutline" />
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
                v-if="t.posterUrl && !failedPosters.has(t.id)"
                :src="posterSrc(t.posterUrl)"
                :alt="t.title"
                loading="lazy"
                @error="onPosterError(t.id)"
              />
              <div v-else class="poster-fallback">{{ t.title.charAt(0) }}</div>
              <span v-if="t.comingSoon" class="badge">Soon</span>
            </div>
            <ion-label class="meta">
              <h3>{{ t.title }}</h3>
              <p>
                <span v-if="t.imdbScore">★ {{ t.imdbScore.toFixed(1) }}</span>
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
        <ion-fab-button size="small" aria-label="Jump to top" @click="scrollTop">
          <ion-icon :icon="arrowUpOutline" />
        </ion-fab-button>
      </ion-fab>
    </ion-content>

    <SourceFilterSheet
      :is-open="filterOpen"
      :filters="filters"
      :sort="sort"
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
.active-filters {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 8px 12px 0;
}
.active-filters .cap {
  text-transform: capitalize;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 12px;
  padding: 12px;
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
  color: var(--ion-text-color, #fff);
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
  color: var(--ion-text-color, #fff);
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
</style>
