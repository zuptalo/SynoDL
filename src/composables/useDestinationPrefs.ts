/**
 * Destination preferences (spec 1011): a default download folder and up to four
 * "favorite" folders. Stored PER-USER on the server so they survive full app
 * closure and stay in sync across the user's sessions. The server also cleans
 * the set on read — a folder that's been deleted on the NAS or whose access the
 * admin revoked simply drops out (and the default resets to root).
 *
 * Module-level refs so every caller shares one reactive source of truth. In
 * legacy/stateless mode (no per-user server storage) the endpoints 404 and this
 * degrades to in-memory-only for the session.
 */
import { ref } from 'vue';
import { api, type DestinationPrefs } from '@/services/api';

export const MAX_FAVORITES = 4;

const defaultDest = ref('');
const favorites = ref<string[]>([]);

function apply(p: DestinationPrefs): void {
  defaultDest.value = p.default ?? '';
  favorites.value = (p.favorites ?? []).slice(0, MAX_FAVORITES);
}

let loaded = false;
async function restore(): Promise<void> {
  if (loaded) return;
  loaded = true;
  try {
    apply(await api.getDestinationPrefs());
  } catch {
    /* stateless / offline — in-memory only for this session */
  }
}

// Persist to the server and adopt the cleaned set it returns (so a favorite the
// server rejected as gone/forbidden disappears locally too).
async function persist(): Promise<void> {
  try {
    apply(await api.setDestinationPrefs({ default: defaultDest.value, favorites: favorites.value }));
  } catch {
    /* best-effort; the in-memory state still drives the UI */
  }
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
