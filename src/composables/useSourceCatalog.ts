/**
 * Reactive state for the download-source catalog (spec 0005). Module-level refs
 * are shared across the Browser tab and its child components (same pattern as
 * useDestinationPrefs): status, the current result page, and the coarse
 * availability states the UI branches on.
 *
 * The catalog is only usable when the server reports a configured, active
 * provider. A 404 (legacy/stateless mode) or a `source_unavailable` /
 * `source_needs_refresh` error degrades to the matching empty state rather than
 * surfacing a raw error.
 */
import { computed, ref } from 'vue';
import {
  api,
  ApiError,
  type CatalogTitle,
  type SourceSearchFilters,
  type SourceStatus,
} from '@/services/api';

const status = ref<SourceStatus | null>(null);
const items = ref<CatalogTitle[]>([]);
const page = ref(1);
const pages = ref(0);
const query = ref('');
const filters = ref<SourceSearchFilters>({});
// Default browse sort when the user hasn't chosen one: most popular, descending.
// Once the user picks a sort/direction (or sets a filter), their choice is
// persisted server-side and loadView() restores it on every open — these
// defaults only fill the gap for a brand-new account with no saved view.
const DEFAULT_SORT = 'favorite';
const DEFAULT_ORDER = 'desc';
const sort = ref(DEFAULT_SORT);
// Sort direction, folded into the same dropdown as the sort field. "desc" is the
// natural reading of every sort option (most popular / newest / highest first).
const order = ref(DEFAULT_ORDER);
const loading = ref(false);
const needsRefresh = ref(false);
const unavailable = ref(true);
const errorMsg = ref('');
const preferredQuality = ref('');

const hasMore = computed(() => page.value < pages.value);
// Sort is now its own always-visible dropdown, so it doesn't count as an active
// "filter" (the clear ✕ and the funnel highlight are about the facet filters).
const hasFilters = computed(() => {
  const f = filters.value;
  return Boolean(
    f.type || f.quality || f.language || f.country || f.score || (f.genre && f.genre.length),
  );
});

async function loadStatus(): Promise<void> {
  try {
    const s = await api.getSourceStatus();
    status.value = s;
    needsRefresh.value = s.state === 'needs_refresh';
    unavailable.value = !s.configured || !s.enabled;
  } catch (e) {
    // A genuine 404 means legacy/stateless mode — the catalog really is
    // unavailable. But a transient transport/5xx error (e.g. a brief blip during
    // a deploy) must NOT masquerade as "no source configured"; keep the last
    // known state so a passing pull-to-refresh / next poll simply recovers.
    if (e instanceof ApiError && e.status === 404) {
      status.value = null;
      unavailable.value = true;
      needsRefresh.value = false;
    }
    // else: leave status/unavailable/needsRefresh untouched.
  }
}

function handleErr(e: unknown): void {
  if (e instanceof ApiError) {
    if (e.code === 'source_needs_refresh') {
      needsRefresh.value = true;
      return;
    }
    if (e.code === 'source_unavailable' || e.status === 404) {
      unavailable.value = true;
      return;
    }
  }
  errorMsg.value = 'Could not reach the source. Try again.';
}

// fetchPage loads the current page and appends (or replaces, when reset) the
// results, enforcing the type filter client-side because the provider's type
// facet is unreliable (browse ignores it; text search has no facets).
// How many titles a "page" should yield after client-side filtering; big enough
// to fill a wide desktop grid so infinite scroll always has something to trigger.
const PAGE_TARGET = 24;

// fetchPage loads the current page and appends (or replaces, when reset) the
// results, dropping upcoming ("coming soon") titles — you can't download those —
// and enforcing the type filter client-side (the provider's type facet doesn't
// apply to text search).
async function fetchPage(reset: boolean): Promise<void> {
  const res = await api.searchSource(query.value, filters.value, page.value, sort.value, order.value);
  needsRefresh.value = false;
  unavailable.value = false;
  pages.value = res.pages;
  let incoming = res.items.filter((i) => !i.comingSoon);
  if (filters.value.type) incoming = incoming.filter((i) => i.type === filters.value.type);
  items.value = reset ? incoming : [...items.value, ...incoming];
}

async function runSearch(reset = true): Promise<void> {
  loading.value = true;
  errorMsg.value = '';
  if (reset) {
    // Keep the current results on screen while the first fresh page loads
    // (fetchPage replaces them on arrival) so a refresh/search doesn't flash to
    // an empty screen. Skeletons only show when there's nothing yet to keep.
    page.value = 1;
  }
  try {
    await fetchPage(reset);
    // Dropping upcoming/type-filtered titles can leave a page thin; pull more
    // pages until we have enough to fill the grid (bounded), so infinite scroll
    // has content to trigger on — important on wide desktop screens.
    let guard = 0;
    while (items.value.length < PAGE_TARGET && page.value < pages.value && guard < 6) {
      page.value += 1;
      guard += 1;
      await fetchPage(false);
    }
  } catch (e) {
    handleErr(e);
  } finally {
    loading.value = false;
  }
}

async function loadMore(): Promise<void> {
  if (!hasMore.value || loading.value) return;
  page.value += 1;
  await runSearch(false);
}

async function setQuery(q: string): Promise<void> {
  query.value = q;
  await runSearch(true);
}

// Persist the current facet filters + sort to the server so the view follows the
// user across devices. Fire-and-forget — a failure never blocks browsing.
async function saveView(): Promise<void> {
  try {
    await api.setSourceView(filters.value, sort.value, order.value);
  } catch {
    /* non-fatal */
  }
}

// Load the saved view (filters + sort + direction) from the server WITHOUT
// searching — the caller reloads the grid. Called on mount and when the app
// returns to the foreground so a change made on another device shows up here.
async function loadView(): Promise<void> {
  try {
    const v = await api.getSourceView();
    filters.value = v.filters ?? {};
    sort.value = v.sort || DEFAULT_SORT;
    order.value = v.order || DEFAULT_ORDER;
  } catch {
    /* non-fatal — keep whatever we have */
  }
}

async function applyFilters(f: SourceSearchFilters, newSort?: string): Promise<void> {
  filters.value = f;
  if (newSort) sort.value = newSort;
  void saveView();
  await runSearch(true);
}

// Change the sort field (from the dropdown beside the search bar) and reload.
async function setSort(s: string): Promise<void> {
  if (s === sort.value) return;
  sort.value = s;
  void saveView();
  await runSearch(true);
}

// Flip the sort direction (the asc/desc toggle inside the same sort control).
async function toggleOrder(): Promise<void> {
  order.value = order.value === 'asc' ? 'desc' : 'asc';
  void saveView();
  await runSearch(true);
}

// Reset every facet filter and reload — the "clear filters" affordance. Sort is
// independent (its own dropdown), so it is left untouched.
async function clearFilters(): Promise<void> {
  filters.value = {};
  void saveView();
  await runSearch(true);
}

// Remove a single active filter (or reset the sort) without opening the sheet.
async function removeFilter(key: keyof SourceSearchFilters | 'sort'): Promise<void> {
  if (key === 'sort') {
    sort.value = DEFAULT_SORT;
    order.value = DEFAULT_ORDER;
  } else {
    const next = { ...filters.value };
    delete next[key];
    filters.value = next;
  }
  void saveView();
  await runSearch(true);
}

async function loadPrefs(): Promise<void> {
  try {
    preferredQuality.value = (await api.getSourcePrefs()).preferredQuality;
  } catch {
    /* non-fatal */
  }
}

async function savePref(q: string): Promise<void> {
  try {
    preferredQuality.value = (await api.setSourcePrefs(q)).preferredQuality;
  } catch {
    /* non-fatal */
  }
}

export function useSourceCatalog() {
  return {
    status,
    items,
    page,
    pages,
    query,
    filters,
    sort,
    order,
    loading,
    needsRefresh,
    unavailable,
    errorMsg,
    hasMore,
    hasFilters,
    preferredQuality,
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
    savePref,
    loadView,
  };
}
