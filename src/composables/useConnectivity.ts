/**
 * Tracks whether the SynoDL server is reachable, so the UI can show a clear
 * "can't reach the server" banner instead of leaving the user guessing (a
 * long-standing pattern in the sibling Ring app).
 *
 * The signal comes from the API layer: every request reports reachable=true when
 * the server responds at all (even a 4xx/5xx), and reachable=false when fetch
 * rejects at the network level. The browser's own offline event is an extra
 * fast-path. Reachability flips back to true on the next successful request (the
 * app polls tasks and reconnects the live stream on their own cadence).
 */
import { ref } from 'vue';
import { CONNECTIVITY_EVENT } from '@/services/api';

const reachable = ref(true);
let wired = false;

function wire(): void {
  if (wired || typeof window === 'undefined') return;
  wired = true;
  window.addEventListener(CONNECTIVITY_EVENT, (e) => {
    reachable.value = (e as CustomEvent<{ reachable: boolean }>).detail.reachable;
  });
  // A browser-level offline state is a definitive "down".
  window.addEventListener('offline', () => {
    reachable.value = false;
  });
  if (typeof navigator !== 'undefined' && navigator.onLine === false) {
    reachable.value = false;
  }
}

export function useConnectivity() {
  wire();
  return { reachable };
}
