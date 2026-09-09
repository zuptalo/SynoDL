/**
 * YouTube downloads (spec 0012, extended by 0013, made live by 1038), held
 * OUTSIDE any component — the same reason uploads are: submitting one from the
 * new-task sheet and then closing that sheet must not hide a download that is
 * still running.
 *
 * Spec 0013 polled this list every five seconds and said why: the existing NAS
 * stream is built around the NAS session, and a five-second poll was judged
 * adequate for a progress bar. It was not — the asking was visible in use, and
 * it is why a rendering fault read as a flicker rather than as a stale row.
 *
 * So this now prefers the stream (GET /v1/ytdl/stream) and falls back to polling
 * whenever the stream is unavailable or drops, exactly as `useTasks` does for
 * NAS tasks. The list is never frozen and never blank.
 *
 * One honest limit, worth knowing before reading further: the server's own
 * picture only advances when its reconciler runs, every three seconds. Streaming
 * removes the asking and delivers a change when it happens; it does not make the
 * underlying state finer-grained.
 *
 * The list is paged, because history is unbounded and one expanded channel can
 * fill it on its own.
 */
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { ApiError, api, streamYtdl, type YtdlDownload, type YtdlUpdate } from '@/services/api';
import { mergeYtdlUpdate } from '@/services/ytdl-merge';

/** Fallback cadence, used ONLY while the stream is down. */
const POLL_MS = 5000;
// Capped exponential backoff between stream reconnect attempts, as useTasks.
const BACKOFF_MS = [1000, 2000, 5000, 10000];

const downloads = ref<YtdlDownload[]>([]);
const degraded = ref(false);
// Unavailable is a normal state, not an error: a deployment without an
// orchestrator answers 503 and the feature simply does not appear.
const available = ref(true);

let pollTimer: ReturnType<typeof setTimeout> | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let controller: AbortController | null = null;
let backoffIdx = 0;
let watchers = 0;

/**
 * Everything that wants to hear about a live update, beyond the list itself.
 *
 * The open group sheet is the reason this exists: its items are PAGED and
 * fetched separately (a channel has no ceiling, so they cannot ride in the list
 * payload), but they must still update without the sheet re-reading itself
 * every few seconds — which was the thing that got noticed. It subscribes here
 * and merges the same delta into its own rows.
 */
const listeners = new Set<(update: YtdlUpdate) => void>();

function applyUpdate(update: YtdlUpdate): void {
  downloads.value = mergeYtdlUpdate(downloads.value, update);
  for (const fn of listeners) fn(update);
}

async function refresh(): Promise<void> {
  try {
    const snap = await api.ytdl();
    downloads.value = snap.downloads ?? [];
    degraded.value = snap.degraded === true;
    available.value = true;
  } catch (e) {
    if (e instanceof ApiError && e.status === 503) {
      available.value = false;
      downloads.value = [];
      return;
    }
    // Anything else: keep the last good list rather than blanking it.
    degraded.value = true;
  }
}

// ---- fallback polling: runs only while the stream is down -------------------
function startPolling(): void {
  if (pollTimer) return;
  const tick = async () => {
    pollTimer = null;
    // Stop the moment the last viewer goes away: a hidden PWA must not keep
    // polling, exactly as the task list does not.
    if (watchers === 0) return;
    if (document.visibilityState === 'visible') await refresh();
    if (watchers > 0) pollTimer = setTimeout(tick, POLL_MS);
  };
  pollTimer = setTimeout(tick, POLL_MS);
}

function stopPolling(): void {
  if (pollTimer) {
    clearTimeout(pollTimer);
    pollTimer = null;
  }
}

// ---- live stream -----------------------------------------------------------
function stopStream(): void {
  if (controller) {
    controller.abort();
    controller = null;
  }
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
}

function scheduleReconnect(): void {
  if (watchers === 0 || reconnectTimer || document.visibilityState !== 'visible') return;
  const delay = BACKOFF_MS[Math.min(backoffIdx, BACKOFF_MS.length - 1)];
  backoffIdx++;
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    void connect();
  }, delay);
}

async function connect(): Promise<void> {
  if (watchers === 0 || controller || document.visibilityState !== 'visible') return;
  const ctrl = new AbortController();
  controller = ctrl;
  try {
    await streamYtdl(
      () => {
        // The list is fetched ON `ready`, not before connecting. A change that
        // lands while the connection is being established is otherwise carried
        // by neither the fetch nor the stream, and would sit wrong until
        // something else moved.
        backoffIdx = 0; // a healthy stream resets the backoff ladder
        stopPolling(); // the stream supersedes the fallback poll
        void refresh();
      },
      applyUpdate,
      ctrl.signal,
    );
    // Resolved: we aborted (hidden/unmounted) or the server closed cleanly —
    // which is also how the server sheds a reader that fell behind.
    if (controller === ctrl) controller = null;
    if (watchers > 0 && !ctrl.signal.aborted && document.visibilityState === 'visible') {
      startPolling();
      scheduleReconnect();
    }
  } catch (e) {
    if (controller === ctrl) controller = null;
    if (e instanceof ApiError && e.code === 'session') {
      // Auth error: the session-expiry flow (router bounce) takes over — do not
      // fall back or reconnect.
      stopStream();
      stopPolling();
      return;
    }
    if (e instanceof ApiError && e.code === 'unavailable') {
      // This deployment does not run YouTube downloads. Not an error and not
      // something to retry at: ask once so the feature can hide itself.
      void refresh();
      return;
    }
    // Anything else — at the connection cap, a proxy in the way, a dropped
    // socket — keeps the list live by polling and retries the stream (FR-008).
    void refresh();
    startPolling();
    scheduleReconnect();
  }
}

function onVisibility(): void {
  if (document.visibilityState === 'visible') {
    void refresh(); // snappy repaint on return
    void connect(); // then go live
  } else {
    stopStream();
    stopPolling();
  }
}

export function useYtdl() {
  onMounted(() => {
    watchers += 1;
    if (watchers === 1) {
      void refresh(); // paint immediately from one fetch
      void connect(); // then upgrade to the live stream
      document.addEventListener('visibilitychange', onVisibility);
    }
  });
  onUnmounted(() => {
    watchers = Math.max(0, watchers - 1);
    if (watchers === 0) {
      stopStream();
      stopPolling();
      document.removeEventListener('visibilitychange', onVisibility);
    }
  });

  async function submit(url: string, mode: 'music' | 'music-video'): Promise<void> {
    await api.ytdlSubmit(url, mode);
    // Not waiting for the stream: the person who just pasted a link should see
    // their row now, not on the next reconcile cycle.
    await refresh();
  }

  /**
   * Retry a failed download. Deliberately NOT optimistic: the server decides
   * whether a retry is allowed at all (only a failed download may be retried),
   * so showing it as queued before it answers would sometimes be a lie.
   */
  async function retry(requestId: string): Promise<void> {
    await api.ytdlRetry(requestId);
    await refresh();
  }

  async function dismiss(requestId: string): Promise<void> {
    // Optimistic: the row goes now, and the stream (or the next poll) is the
    // source of truth.
    downloads.value = downloads.value.filter((d) => d.requestId !== requestId);
    try {
      await api.ytdlDismiss(requestId);
    } catch {
      await refresh(); // it did not go after all — put it back
    }
  }

  return {
    downloads: computed(() => downloads.value),
    degraded: computed(() => degraded.value),
    available: computed(() => available.value),
    refresh,
    submit,
    dismiss,
    retry,
  };
}

/**
 * Subscribe to live updates for something other than the top-level list.
 *
 * Returns an unsubscribe. Used by the open group sheet, whose items are fetched
 * and paged separately but must move without re-reading themselves.
 */
export function onYtdlUpdate(fn: (update: YtdlUpdate) => void): () => void {
  listeners.add(fn);
  return () => listeners.delete(fn);
}
