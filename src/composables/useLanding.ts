/**
 * The tab the app opens to after launch/sign-in. A per-device preference with
 * **Discover as the default** (the catalog is the primary way most people start a
 * download). Persisted to localStorage so the router can read it synchronously
 * when resolving the initial redirect — no flash of the wrong tab. Mirrors the
 * pattern in useTheme.
 */
import { ref } from 'vue';

export type Landing = 'discover' | 'tasks';

const KEY = 'landing.page';
// Route paths for each choice. Discover is the BrowserPage route.
const PATHS: Record<Landing, string> = {
  discover: '/tabs/browser',
  tasks: '/tabs/tasks',
};

function read(): Landing {
  try {
    return localStorage.getItem(KEY) === 'tasks' ? 'tasks' : 'discover';
  } catch {
    return 'discover';
  }
}

// Module-level singleton so the router and the settings UI share one source.
const landing = ref<Landing>(read());

/** The route path to land on — safe to call from the router at module load. */
export function landingPath(): string {
  return PATHS[landing.value] ?? PATHS.discover;
}

export function useLanding() {
  function setLanding(value: Landing): void {
    landing.value = value;
    try {
      localStorage.setItem(KEY, value);
    } catch {
      /* private mode — the in-memory ref still drives this session */
    }
  }
  return { landing, setLanding, landingPath };
}
