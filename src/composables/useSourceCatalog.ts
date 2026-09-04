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
  type DegradedSource,
  type SourceParameters,
  type SourceProvider,
  type SourceSearchFilters,
  type SourceStatus,
} from '@/services/api';
import { COUNTRIES, GENRES, LANGUAGES, QUALITIES, SCORES, SORTS, TYPES } from '@/services/source-filters';
import {
  countryOptions,
  genreOptions,
  languageOptions,
  passthroughOptions,
  qualityOptions,
  scoreOptions,
  sortOptions,
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
// Cross-tab "Open in Discover" handoff (spec 1016): the Tasks detail sets one of
// these and navigates to the Browser tab, which consumes it. A title object opens
// that exact title's modal; a query string runs a search fallback (used when a task
// has no stored catalog id).
const pendingOpen = ref<CatalogTitle | null>(null);
const pendingSearch = ref('');
function requestOpen(title: CatalogTitle | null, searchQuery?: string): void {
  pendingOpen.value = title;
  pendingSearch.value = title ? '' : (searchQuery ?? '');
}
// The configured sources, and which one the user is looking at. '' means "All
// sources" — the default, and what an install with a single source always uses.
const sources = ref<SourceProvider[]>([]);
const selectedSource = ref('');
/**
 * Hide titles the user already has, or is already fetching.
 *
 * Downloading counts as hidden too (FR-019a): it is not something they need to
 * send again, and its progress is reported in the Tasks list. Remembered per user
 * so the choice follows them across devices.
 */
const hideOwned = ref(false);
// Shown only once there is a real choice to make: with one source a selector
// would be a control with nothing to select (FR-013).
const showSourcePicker = computed(() => sources.value.length > 1);
const selectedSourceName = computed(
  () => sources.value.find((s) => String(s.id) === selectedSource.value)?.displayName ?? 'All sources',
);
// Sources that dropped out of the last query. Rendered as a non-blocking notice:
// a failing source must never blank results the healthy ones could fill.
const degraded = ref<DegradedSource[]>([]);

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
//
// Every facet list is defaulted to [] before it is mapped. Our own server always
// sends each one as an array, but this runs inside a computed: one missing key
// would throw on .map, and a computed that throws during render takes the whole
// Discover page down (blank grid, no chips, no filter button) rather than just
// emptying one dropdown. A missing facet degrades to "no options" instead.
const filterOptions = computed(() => {
  const p = parameters.value;
  return {
    types: withAny(p ? typeOptions(p.types ?? []) : TYPES, 'Any type'),
    genres: withAny(p ? genreOptions(p.genres ?? []) : GENRES, 'Any genre'),
    qualities: withAny(p ? qualityOptions(p.qualities ?? []) : qualityStatic, 'Any quality'),
    scores: withAny(p ? scoreOptions(p.scores ?? []) : SCORES, 'Any rating'),
    languages: withAny(p ? languageOptions(p.languages ?? []) : LANGUAGES, 'Any language'),
    countries: withAny(p ? countryOptions(p.countries ?? []) : COUNTRIES, 'Any country'),
    // Advanced facets only exist when the live parameters are loaded.
    channels: withAny(p ? passthroughOptions(p.channels ?? []) : [], 'Any channel'),
    encoders: withAny(p ? passthroughOptions(p.encoders ?? []) : [], 'Any encoder'),
    ages: withAny(p ? passthroughOptions(p.ages ?? []) : [], 'Any rating'),
    // Sorts take no "Any" entry — a browse is always in some order. The built-in
    // list stands in only until the live capabilities arrive; after that the
    // control offers exactly what the selected source(s) can honour, so it can
    // never present an ordering that would silently fall back to the default
    // (spec 1024, FR-002/FR-008).
    sorts: p?.sorts?.length ? sortOptions(p.sorts) : SORTS,
  };
});

// Keep the chosen ordering honest when the offered set changes — switching from
// one source to another, or to combined, can take the current ordering away.
// Leaving it selected would show a sort the results are not actually in.
function reconcileSort(): void {
  const offered = filterOptions.value.sorts;
  if (!offered.length || offered.some((o) => o.value === sort.value)) return;
  sort.value = offered.some((o) => o.value === DEFAULT_SORT) ? DEFAULT_SORT : offered[0].value;
}
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
    parameters.value = await api.getSourceParameters(selectedSource.value);
    reconcileSort();
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

// True when the view differs from a fresh account's in ANY way — a filter, or a
// sort/order the user changed. hasFilters alone is not enough for the reset
// affordance: a user who has only changed the sort has just as much to undo,
// and would otherwise be left with no one-tap way back.
const viewChanged = computed(
  () => hasFilters.value || sort.value !== DEFAULT_SORT || order.value !== DEFAULT_ORDER,
);

// The source list drives the picker and the per-result labels. It is admin-only
// on the server, so a non-admin simply gets no list and sees no picker — which
// is correct: a non-admin cannot add sources anyway, and the labels come with
// each result.
async function loadSources(): Promise<void> {
  try {
    sources.value = (await api.listSourceProviders()).providers.filter((p) => p.enabled);
  } catch {
    sources.value = [];
  }
}

async function setSource(id: string): Promise<string[]> {
  if (selectedSource.value === id) return [];
  selectedSource.value = id;
  // Facets differ per source: combined mode offers only what every source
  // understands, so the sheet must be refetched, and any filter the new
  // selection cannot honour has to go rather than silently doing nothing.
  await loadParameters();
  const dropped = dropUnsupportedFilters();
  await saveView();
  await runSearch();
  return dropped;
}

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
  const res = await api.searchSource(
    query.value,
    filters.value,
    page.value,
    sort.value,
    order.value,
    selectedSource.value,
  );
  needsRefresh.value = false;
  unavailable.value = false;
  pages.value = res.pages;
  degraded.value = res.degraded ?? [];
  // Filtered in the same place comingSoon is, so pagination and the backfill
  // below behave identically whichever reason a card was removed for.
  const incoming = res.items.filter(
    (i) => !i.comingSoon && !(hideOwned.value && (i.ownership === 'owned' || i.ownership === 'downloading')),
  );
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

// How many pages one infinite-scroll trigger pulls. Fetching a page BEYOND the
// one the user is about to reach keeps a fast scroller from meeting the spinner
// at the bottom of every page — the grid stays a screen ahead instead of
// catching up. This doesn't cost the provider more requests for a given amount
// of scrolling; the same pages are fetched, just sooner and in pairs.
const PAGES_PER_LOAD = 2;

async function loadMore(): Promise<void> {
  // Stop paginating the moment the source is down — otherwise a failing provider
  // gets hammered with rapid follow-up page requests (which, against a Cloudflare
  // rate-limit, only makes it worse). Re-checked between pages too, so a source
  // that starts failing on the first one never gets asked for the second.
  for (let i = 0; i < PAGES_PER_LOAD; i += 1) {
    if (!hasMore.value || needsRefresh.value || unavailable.value || errorMsg.value) return;
    // Only the first pass guards on `loading`: it's the re-entrancy check for a
    // second trigger arriving mid-load. Inside the loop we ARE the load.
    if (i === 0 && loading.value) return;
    page.value += 1;
    await runSearch(false);
  }
}

/**
 * Turn hiding on or off, persist it, and reload the grid.
 *
 * A full reload rather than filtering what is already loaded: the hidden cards
 * would otherwise leave gaps, and the backfill that keeps the grid full only
 * runs on a fetch.
 */
async function setHideOwned(on: boolean): Promise<void> {
  hideOwned.value = on;
  void saveView();
  await runSearch(true);
}

async function setQuery(q: string): Promise<void> {
  query.value = q;
  await runSearch(true);
}

// Persist the current facet filters + sort to the server so the view follows the
// user across devices. Fire-and-forget — a failure never blocks browsing.
async function saveView(): Promise<void> {
  try {
    await api.setSourceView(filters.value, sort.value, order.value, selectedSource.value, hideOwned.value);
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
    // The server normalizes a removed or disabled source back to "all", so a
    // stale selection lands the user somewhere sensible rather than on a dead view.
    selectedSource.value = v.selectedSource ?? '';
    hideOwned.value = v.hideOwned ?? false;
  } catch {
    /* non-fatal — keep whatever we have */
  }
}

// Drop any active filter the current selection cannot honour, so switching back
// to "All sources" never leaves a filter applied that only one source
// understood — the user would see it in the chips and reasonably assume it was
// still narrowing the results (FR-016). Returns the keys it removed so the UI
// can say what happened rather than silently changing the results.
function dropUnsupportedFilters(): string[] {
  const opts = filterOptions.value;
  const supported = (key: keyof SourceSearchFilters, list: Option[]): boolean =>
    list.some((o) => o.value !== '' && o.value === filters.value[key]);
  const dropped: string[] = [];
  const checks: Array<[keyof SourceSearchFilters, Option[]]> = [
    ['quality', opts.qualities],
    ['language', opts.languages],
    ['country', opts.countries],
    ['score', opts.scores],
    ['channel', opts.channels],
    ['encoder', opts.encoders],
  ];
  for (const [key, list] of checks) {
    if (filters.value[key] && list.length > 0 && !supported(key, list)) {
      delete filters.value[key];
      dropped.push(String(key));
    }
  }
  if (filters.value.genre?.length && opts.genres.length > 0) {
    const keep = filters.value.genre.filter((g) => opts.genres.some((o) => o.value === g));
    if (keep.length !== filters.value.genre.length) dropped.push('genre');
    if (keep.length) filters.value.genre = keep;
    else delete filters.value.genre;
  }
  if (dropped.length) filters.value = { ...filters.value };
  return dropped;
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

// Put the whole view back to default — filters AND sort — in one round trip.
// Clearing the filters and resetting the sort separately would run two searches
// and flash an intermediate result the user never asked for.
async function resetView(): Promise<void> {
  filters.value = {};
  sort.value = DEFAULT_SORT;
  order.value = DEFAULT_ORDER;
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
    pendingOpen,
    pendingSearch,
    requestOpen,
    loading,
    needsRefresh,
    unavailable,
    errorMsg,
    hasMore,
    hasFilters,
    viewChanged,
    preferredQuality,
    sources,
    selectedSource,
    selectedSourceName,
    hideOwned,
    setHideOwned,
    showSourcePicker,
    degraded,
    loadSources,
    setSource,
    dropUnsupportedFilters,
    loadStatus,
    runSearch,
    loadMore,
    setQuery,
    applyFilters,
    setSort,
    toggleOrder,
    clearFilters,
    resetView,
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
