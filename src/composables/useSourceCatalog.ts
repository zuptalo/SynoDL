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
// Default browse sort: release year, descending (newest first).
const DEFAULT_SORT = 'year';
const sort = ref(DEFAULT_SORT);
const loading = ref(false);
const needsRefresh = ref(false);
const unavailable = ref(true);
const errorMsg = ref('');
const preferredQuality = ref('');

const hasMore = computed(() => page.value < pages.value);
const hasFilters = computed(() => {
  const f = filters.value;
  return Boolean(
    f.type ||
      f.quality ||
      f.language ||
      f.country ||
      f.score ||
      (f.genre && f.genre.length) ||
      sort.value !== DEFAULT_SORT,
  );
});

async function loadStatus(): Promise<void> {
  try {
    const s = await api.getSourceStatus();
    status.value = s;
    needsRefresh.value = s.state === 'needs_refresh';
    unavailable.value = !s.configured || !s.enabled;
  } catch {
    // 404 (legacy mode) or a transport error → treat the catalog as unavailable.
    status.value = null;
    unavailable.value = true;
    needsRefresh.value = false;
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
async function fetchPage(reset: boolean): Promise<void> {
  const res = await api.searchSource(query.value, filters.value, page.value, sort.value);
  needsRefresh.value = false;
  unavailable.value = false;
  pages.value = res.pages;
  const wantType = filters.value.type;
  const incoming = wantType ? res.items.filter((i) => i.type === wantType) : res.items;
  items.value = reset ? incoming : [...items.value, ...incoming];
}

async function runSearch(reset = true): Promise<void> {
  loading.value = true;
  errorMsg.value = '';
  if (reset) {
    page.value = 1;
    items.value = [];
  }
  try {
    await fetchPage(reset);
    // A client-side type filter can leave a page nearly empty; pull a few more
    // pages so the grid fills while results still exist (bounded).
    let guard = 0;
    while (filters.value.type && items.value.length < 12 && page.value < pages.value && guard < 4) {
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

async function applyFilters(f: SourceSearchFilters, newSort?: string): Promise<void> {
  filters.value = f;
  if (newSort) sort.value = newSort;
  await runSearch(true);
}

// Reset every filter and the sort back to the default (latest releases,
// descending) and reload — the "clear filters" affordance.
async function clearFilters(): Promise<void> {
  filters.value = {};
  sort.value = DEFAULT_SORT;
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
    clearFilters,
    loadPrefs,
    savePref,
  };
}
