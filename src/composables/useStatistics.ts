/**
 * Download statistics (spec 0006). Loads server-aggregated per-user summaries
 * and the daily download time-series for the Statistics section. Read-only.
 *
 * Module-level refs so the modal and its children share one reactive source of
 * truth. In legacy/stateless mode the endpoints aren't available and this
 * degrades quietly (available = false) so the section can hide itself.
 */
import { ref } from 'vue';
import { ApiError, api, type StatSource, type StatUserSummary } from '@/services/api';

const summary = ref<StatUserSummary[]>([]);
const days = ref<{ date: string; count: number }[]>([]);
const available = ref(true);
const loadingSummary = ref(false);
const loadingSeries = ref(false);

async function loadSummary(): Promise<void> {
  loadingSummary.value = true;
  try {
    summary.value = (await api.getStatsSummary()).users;
    available.value = true;
  } catch (e) {
    // 403 in legacy/stateless mode — statistics don't apply there.
    if (e instanceof ApiError && e.status === 403) available.value = false;
  } finally {
    loadingSummary.value = false;
  }
}

async function loadTimeseries(opts: { source?: StatSource; userId?: number | 'all' } = {}): Promise<void> {
  loadingSeries.value = true;
  try {
    days.value = (await api.getStatsTimeseries(opts)).days;
  } catch {
    days.value = [];
  } finally {
    loadingSeries.value = false;
  }
}

export function useStatistics() {
  return { summary, days, available, loadingSummary, loadingSeries, loadSummary, loadTimeseries };
}
