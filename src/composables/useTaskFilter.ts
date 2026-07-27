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
}

const state = ref<TaskFilterState>(defaultTaskFilter());
let restored = false;

async function restore(): Promise<void> {
  if (restored) return;
  restored = true;
  try {
    const row = await get<FilterRow>('settings', 'taskFilter');
    if (row) {
      const { id: _id, ...rest } = row;
      // Merge over defaults so a filter saved by an older build (fewer keys)
      // still yields a complete state.
      state.value = { ...defaultTaskFilter(), ...rest };
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
      await put<FilterRow>('settings', { id: 'taskFilter', ...next });
    } catch {
      /* private mode — state still applies for this session */
    }
  }

  return { filter: state, apply };
}
