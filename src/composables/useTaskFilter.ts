/**
 * Persisted task filter/sort state (spec 0001 US5, FR-017). Stored as the
 * `taskFilter` row of the idb `settings` store; module-level refs so the
 * sheet and the list share one live instance.
 */
import { ref } from 'vue';
import { get, put } from '@/db/idb';
import { defaultTaskFilter, type TaskFilterState } from '@/services/task-sort';

interface FilterRow extends TaskFilterState {
  id: 'taskFilter';
  // The default order changed in spec 1048. A row saved before then carries
  // the OLD default as if the user had chosen it, so the version says which
  // default it was saved under; a row without one is migrated once.
  v?: number;
}

const FILTER_VERSION = 2;

const state = ref<TaskFilterState>(defaultTaskFilter());
let restored = false;

async function restore(): Promise<void> {
  if (restored) return;
  restored = true;
  try {
    const row = await get<FilterRow>('settings', 'taskFilter');
    if (row) {
      const { id: _id, v, ...rest } = row;
      // Merge over defaults so a filter saved by an older build (fewer keys)
      // still yields a complete state.
      const next = { ...defaultTaskFilter(), ...rest };
      // A row from before the default changed, still on the old default: it
      // is the old default, not a choice, so it takes the new one. A sort the
      // user actually picked is left alone.
      if ((v ?? 1) < FILTER_VERSION && rest.sortKey === 'createdAt' && !rest.ascending) {
        next.sortKey = defaultTaskFilter().sortKey;
      }
      state.value = next;
    }
  } catch {
    // Private mode: defaults only, nothing persists.
  }
}

export function useTaskFilter() {
  void restore();

  async function apply(next: TaskFilterState): Promise<void> {
    state.value = next;
    try {
      await put<FilterRow>('settings', { id: 'taskFilter', v: FILTER_VERSION, ...next });
    } catch {
      /* private mode — state still applies for this session */
    }
  }

  return { filter: state, apply };
}
