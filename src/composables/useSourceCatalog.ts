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
  type SourceParameters,
  type SourceSearchFilters,
  type SourceStatus,
} from '@/services/api';
import { COUNTRIES, GENRES, LANGUAGES, QUALITIES, SCORES, TYPES } from '@/services/source-filters';
import {
  countryOptions,
  genreOptions,
  languageOptions,
  passthroughOptions,
  qualityOptions,
  scoreOptions,
  typeOptions,
  type Option,
} from '@/services/facet-labels';

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
// True while a text query is in play. The source cannot sort or facet-filter
// text-search results (only the Type filter is honored, via each result's
// title_type) — so the UI disables the sort control and the non-type filters
// while this is true, rather than letting them silently no-op (spec 2002).
const searchActive = computed(() => query.value.trim() !== '');
// True when a search is active AND the user has a selection the source will
// ignore for text search: any non-type facet filter, or a non-default sort/order.
// Drives the "these selections do nothing right now" affordances — the strike-through
// on the chips/sort and the conditional "clear the search" nudge (spec 1014).
const searchIneffective = computed(() => {
  if (!searchActive.value) return false;
  const f = filters.value;
  const hasNonTypeFilter = Boolean(
    f.quality ||
      f.language ||
      f.country ||
      f.score ||
      (f.genre && f.genre.length) ||
      f.age ||
      f.channel ||
      f.encoder ||
      f.x265 ||
      f.threeD ||
      f.cast ||
      f.director ||
      f.creator ||
      f.yearFrom ||
      f.yearTo,
  );
  const nonDefaultSort = sort.value !== DEFAULT_SORT || order.value !== DEFAULT_ORDER;
  return hasNonTypeFilter || nonDefaultSort;
});
const loading = ref(false);
const needsRefresh = ref(false);
const unavailable = ref(true);
const errorMsg = ref('');
const preferredQuality = ref('');

// The provider's live filter facets (fetched on open/foreground). Null until
// loaded — the UI then falls back to the built-in lists.
const parameters = ref<SourceParameters | null>(null);

// Ensure a single leading "Any" option, whether the source list has one or not.
function withAny(opts: Option[], anyLabel: string): Option[] {
  return [{ value: '', label: anyLabel }, ...opts.filter((o) => o.value !== '')];
}
const qualityStatic: Option[] = QUALITIES.map((q) => ({ value: q, label: q }));

// Filter options for the sheet + chips: the provider's live facets when loaded,
// otherwise the built-in lists. Each carries a leading "Any" entry.
const filterOptions = computed(() => {
  const p = parameters.value;
  return {
    types: withAny(p ? typeOptions(p.types) : TYPES, 'Any type'),
    genres: withAny(p ? genreOptions(p.genres) : GENRES, 'Any genre'),
    qualities: withAny(p ? qualityOptions(p.qualities) : qualityStatic, 'Any quality'),
    scores: withAny(p ? scoreOptions(p.scores) : SCORES, 'Any rating'),
    languages: withAny(p ? languageOptions(p.languages) : LANGUAGES, 'Any language'),
    countries: withAny(p ? countryOptions(p.countries) : COUNTRIES, 'Any country'),
    // Advanced facets only exist when the live parameters are loaded.
    channels: withAny(p ? passthroughOptions(p.channels) : [], 'Any channel'),
    encoders: withAny(p ? passthroughOptions(p.encoders) : [], 'Any encoder'),
    ages: withAny(p ? passthroughOptions(p.ages) : [], 'Any rating'),
  };
});
// Year bounds for the range inputs (fall back to a sensible window).
const yearBounds = computed(() => ({
  min: parameters.value?.minYear || 1900,
  max: parameters.value?.maxYear || new Date().getFullYear() + 1,
}));

// Resolve a facet value to its human label (for the active-filter chips).
function optionLabel(list: Option[], value?: string): string {
  if (!value) return '';
  return list.find((o) => o.value === value)?.label ?? value;
}

// Refresh the provider's facet lists. Fire-and-forget — a failure just keeps the
// built-in lists. Called on open and when the app returns to the foreground.
async function loadParameters(): Promise<void> {
  try {
    parameters.value = await api.getSourceParameters();
  } catch {
    /* keep whatever we have (built-in lists cover it) */
  }
}

const hasMore = computed(() => page.value < pages.value);
// Sort is now its own always-visible dropdown, so it doesn't count as an active
// "filter" (the clear ✕ and the funnel highlight are about the facet filters).
const hasFilters = computed(() => {
  const f = filters.value;
  return Boolean(
    f.type ||
      f.quality ||
      f.language ||
      f.country ||
      f.score ||
      (f.genre && f.genre.length) ||
      f.age ||
      f.channel ||
      f.encoder ||
      f.x265 ||
      f.threeD ||
      f.cast ||
      f.director ||
      f.creator ||
      f.yearFrom ||
      f.yearTo,
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
    // Transient: the source rate-limited or blipped on one request while others
    // still succeed. Keep the session — don't show the "needs refreshing" screen.
    if (e.code === 'source_busy') {
      errorMsg.value = 'The source is busy — give it a moment and try again.';
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
// results, dropping upcoming ("coming soon") titles — you can't download those.
// Type filtering is done SERVER-SIDE (the provider's type code for browse, and
// the full_search path for text search); there's no client-side type re-filter,
// because the filter value is now the provider's numeric code (e.g. "15") which
// never equals a result's type string ("movie") — that mismatch was dropping
// every result.
async function fetchPage(reset: boolean): Promise<void> {
  const res = await api.searchSource(query.value, filters.value, page.value, sort.value, order.value);
  needsRefresh.value = false;
  unavailable.value = false;
  pages.value = res.pages;
  const incoming = res.items.filter((i) => !i.comingSoon);
  items.value = reset ? incoming : [...items.value, ...incoming];
}

// A rolling deploy makes the backend briefly unreachable. Retry a transient
// transport failure (a rejected fetch, or a 5xx) a few times before surfacing
// the hard error, so the catalog rides out an update instead of forcing the user
// to pull-to-refresh repeatedly. A real ApiError (needs-refresh, unavailable) is
// not retried — it's a definite answer.
async function fetchWithRetry(reset: boolean): Promise<void> {
  const attempts = 4;
  for (let i = 0; ; i += 1) {
    try {
      await fetchPage(reset);
      return;
    } catch (e) {
      const transient = !(e instanceof ApiError) || e.status >= 500;
      if (!transient || i >= attempts - 1) throw e;
      await new Promise((r) => setTimeout(r, 1000));
    }
  }
}

// Each fresh search (reset) gets a new generation. If a newer one starts while an
// older is still paginating, the older bails at its next checkpoint — so we never
// keep hitting the provider for a filter combo the user already moved past.
let searchGen = 0;

async function runSearch(reset = true): Promise<void> {
  const gen = reset ? ++searchGen : searchGen;
  loading.value = true;
  errorMsg.value = '';
  if (reset) {
    // Keep the current results on screen while the first fresh page loads
    // (fetchPage replaces them on arrival) so a refresh/search doesn't flash to
    // an empty screen. Skeletons only show when there's nothing yet to keep.
    page.value = 1;
  }
  try {
    await fetchWithRetry(reset);
    if (gen !== searchGen) return; // superseded by a newer search
    // Dropping upcoming/type-filtered titles can leave a page thin; pull more
    // pages until we have enough to fill the grid (bounded), so infinite scroll
    // has content to trigger on — important on wide desktop screens.
    let guard = 0;
    while (items.value.length < PAGE_TARGET && page.value < pages.value && guard < 6) {
      page.value += 1;
      guard += 1;
      await fetchPage(false);
      if (gen !== searchGen) return; // superseded mid-pagination — stop calling the provider
    }
  } catch (e) {
    if (gen === searchGen) handleErr(e); // ignore errors from a superseded search
  } finally {
    if (gen === searchGen) loading.value = false;
  }
}

// Coalesce rapid filter/sort changes into ONE search: each change restarts a
// short timer and only the final combo is requested — no request per keystroke or
// toggle. Callers await the shared promise so post-search UI (scroll-to-top,
// viewport fill) still runs once the results are in.
let debounceTimer: ReturnType<typeof setTimeout> | null = null;
let debounceWaiters: Array<() => void> = [];
const SEARCH_DEBOUNCE_MS = 300;

function debouncedSearch(): Promise<void> {
  return new Promise((resolve) => {
    debounceWaiters.push(resolve);
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      debounceTimer = null;
      const waiters = debounceWaiters;
      debounceWaiters = [];
      void runSearch(true).finally(() => waiters.forEach((w) => w()));
    }, SEARCH_DEBOUNCE_MS);
  });
}

async function loadMore(): Promise<void> {
  // Stop paginating the moment the source is down — otherwise a failing provider
  // gets hammered with rapid follow-up page requests (which, against a Cloudflare
  // rate-limit, only makes it worse).
  if (!hasMore.value || loading.value || needsRefresh.value || unavailable.value || errorMsg.value) return;
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
  await debouncedSearch();
}

// Change the sort field (from the dropdown beside the search bar) and reload.
async function setSort(s: string): Promise<void> {
  if (s === sort.value) return;
  sort.value = s;
  void saveView();
  await debouncedSearch();
}

// Flip the sort direction (the asc/desc toggle inside the same sort control).
async function toggleOrder(): Promise<void> {
  order.value = order.value === 'asc' ? 'desc' : 'asc';
  void saveView();
  await debouncedSearch();
}

// Reset every facet filter and reload — the "clear filters" affordance. Sort is
// independent (its own dropdown), so it is left untouched.
async function clearFilters(): Promise<void> {
  filters.value = {};
  void saveView();
  await debouncedSearch();
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
  await debouncedSearch();
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
    searchActive,
    searchIneffective,
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
    filterOptions,
    yearBounds,
    optionLabel,
    loadParameters,
  };
}
