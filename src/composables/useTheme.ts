/**
 * Appearance theme: a manual dark/light choice with **dark as the default** and
 * deliberately NO "follow the system" option (product decision). The choice is
 * persisted to localStorage so the synchronous pre-paint script in index.html
 * can apply the dark palette class before Vue mounts (no light flash on a cold
 * launch). localStorage (not IndexedDB) because the pre-paint must read it
 * synchronously.
 */
import { ref } from 'vue';

export type Theme = 'dark' | 'light';

const KEY = 'appearance.theme';

function read(): Theme {
  try {
    return localStorage.getItem(KEY) === 'light' ? 'light' : 'dark';
  } catch {
    return 'dark';
  }
}

// Module-level singleton so every caller shares one reactive source of truth.
const theme = ref<Theme>(read());

function apply(t: Theme): void {
  const dark = t !== 'light';
  document.documentElement.classList.toggle('ion-palette-dark', dark);
  try {
    localStorage.setItem(KEY, t);
  } catch {
    /* private mode / storage disabled — the in-memory ref still drives the UI */
  }
}

// Keep the runtime class in sync with the stored preference from first import.
apply(theme.value);

export function useTheme() {
  function setTheme(t: Theme): void {
    theme.value = t;
    apply(t);
  }
  function toggle(): void {
    setTheme(theme.value === 'dark' ? 'light' : 'dark');
  }
  return { theme, setTheme, toggle };
}
