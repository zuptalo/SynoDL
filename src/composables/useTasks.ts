/**
 * Task-list live sync. The NAS is the source of truth (constitution Principle
 * IV); this composable prefers the SSE stream (GET /v1/tasks/stream) for a
 * live-feeling UI and falls back to 3s polling whenever the stream is
 * unavailable or drops — so the list is never frozen. Both transports run only
 * while the owner view is mounted AND the tab is visible: a hidden PWA must not
 * keep the NAS awake.
 */
import { onMounted, onUnmounted, ref } from 'vue';
import { ApiError, api, streamTasks, type TaskSnapshot } from '@/services/api';
import type { Stats, Task } from '@/types/task';

const POLL_MS = 3000;
// Capped exponential backoff between stream reconnect attempts.
const BACKOFF_MS = [1000, 2000, 5000, 10000];

export function useTasks() {
  const tasks = ref<Task[]>([]);
  const stats = ref<Stats>({ downloadSpeed: 0, uploadSpeed: 0 });
  const loaded = ref(false);
  const error = ref('');

  let stopped = false;
  let pollTimer: ReturnType<typeof setTimeout> | null = null;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let controller: AbortController | null = null;
  let backoffIdx = 0;

  function applySnapshot(snap: TaskSnapshot): void {
    tasks.value = snap.tasks;
    stats.value = snap.stats;
    error.value = '';
    loaded.value = true;
  }

  async function refresh(): Promise<void> {
    try {
      applySnapshot(await api.tasks());
    } catch (e) {
      // Session expiry is handled globally (api dispatches, router bounces);
      // everything else surfaces inline without clearing the last good list.
      error.value = e instanceof Error ? e.message : 'error';
    } finally {
      loaded.value = true;
    }
  }

  // ---- fallback polling: runs only while the stream is down ----------------
  function startPolling(): void {
    if (pollTimer) return;
    const tick = async () => {
      if (stopped) return;
      if (document.visibilityState === 'visible') await refresh();
      pollTimer = setTimeout(tick, POLL_MS);
    };
    pollTimer = setTimeout(tick, POLL_MS);
  }
  function stopPolling(): void {
    if (pollTimer) {
      clearTimeout(pollTimer);
      pollTimer = null;
    }
  }

  // ---- live stream ---------------------------------------------------------
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
    if (stopped || reconnectTimer || document.visibilityState !== 'visible') return;
    const delay = BACKOFF_MS[Math.min(backoffIdx, BACKOFF_MS.length - 1)];
    backoffIdx++;
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null;
      void connect();
    }, delay);
  }

  async function connect(): Promise<void> {
    if (stopped || controller || document.visibilityState !== 'visible') return;
    const ctrl = new AbortController();
    controller = ctrl;
    try {
      await streamTasks((snap) => {
        backoffIdx = 0; // a healthy stream resets the backoff ladder
        stopPolling(); // a live snapshot supersedes the fallback poll
        applySnapshot(snap);
      }, ctrl.signal);
      // Resolved: caller aborted (hidden/unmounted) or the server closed cleanly.
      if (controller === ctrl) controller = null;
      if (!stopped && !ctrl.signal.aborted && document.visibilityState === 'visible') {
        startPolling();
        scheduleReconnect();
      }
    } catch (e) {
      if (controller === ctrl) controller = null;
      if (e instanceof ApiError && e.code === 'session') {
        // Auth error: the session-expiry flow (router bounce) takes over — do
        // not fall back or reconnect.
        stopStream();
        stopPolling();
        return;
      }
      // Transport failure: keep the UI live via polling and retry the stream.
      void refresh(); // immediate, don't wait a poll tick
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

  onMounted(() => {
    void refresh(); // paint immediately from one poll
    void connect(); // then upgrade to the live stream
    document.addEventListener('visibilitychange', onVisibility);
  });
  onUnmounted(() => {
    stopped = true;
    stopStream();
    stopPolling();
    document.removeEventListener('visibilitychange', onVisibility);
  });

  return { tasks, stats, loaded, error, refresh };
}
