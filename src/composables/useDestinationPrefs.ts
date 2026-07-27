/**
 * Destination preferences (spec 1006): a persisted default download folder and
 * up to four "favorite" folders shown as quick-select chips in the new-task
 * sheet. Stored in the idb `settings` store (offline-first) under one row;
 * module-level refs so every caller shares the same reactive state.
 */
import { ref } from 'vue';
import { get, put } from '@/db/idb';

export const MAX_FAVORITES = 4;

interface DestinationRow {
  id: 'destinationPrefs';
  default: string;
  favorites: string[];
}

const defaultDest = ref('');
const favorites = ref<string[]>([]);

let restored = false;
async function restore(): Promise<void> {
  if (restored) return;
  restored = true;
  try {
    const row = await get<DestinationRow>('settings', 'destinationPrefs');
    if (row) {
      defaultDest.value = row.default ?? '';
      favorites.value = (row.favorites ?? []).slice(0, MAX_FAVORITES);
    }
  } catch {
    /* offline / no row yet — defaults stand */
  }
}

async function persist(): Promise<void> {
  await put<DestinationRow>('settings', {
    id: 'destinationPrefs',
    default: defaultDest.value,
    favorites: favorites.value,
  });
}

export function useDestinationPrefs() {
  void restore();

  function setDefault(dest: string): void {
    defaultDest.value = dest;
    void persist();
  }

  function isFavorite(dest: string): boolean {
    return favorites.value.includes(dest);
  }

  /** Add a favorite (capped at MAX_FAVORITES) or remove it if already present. */
  function toggleFavorite(dest: string): void {
    if (!dest) return;
    if (isFavorite(dest)) {
      favorites.value = favorites.value.filter((f) => f !== dest);
    } else if (favorites.value.length < MAX_FAVORITES) {
      favorites.value = [...favorites.value, dest];
    }
    void persist();
  }

  return { defaultDest, favorites, setDefault, isFavorite, toggleFavorite };
}
