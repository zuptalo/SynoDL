/**
 * YouTube downloads (spec 0012), held OUTSIDE any component — the same reason
 * uploads are: submitting one from the new-task sheet and then closing that
 * sheet must not hide a download that is still running.
 *
 * There is deliberately no progress here. A download reports only where it is
 * in its life — scheduled, started, completed, failed — because that is what
 * was asked for, and because the worker has no channel back to us anyway. That
 * makes polling cheap: one request, whatever the history size.
 */
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { ApiError, api, type YtdlDownload } from '@/services/api';

const POLL_MS = 5000;

const downloads = ref<YtdlDownload[]>([]);
const degraded = ref(false);
// Unavailable is a normal state, not an error: a deployment without an
// orchestrator answers 503 and the feature simply does not appear.
const available = ref(true);

let timer: ReturnType<typeof setTimeout> | null = null;
let watchers = 0;

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

function schedule(): void {
  if (timer) return;
  timer = setTimeout(async () => {
    timer = null;
    // Stop the moment the last viewer goes away: a hidden PWA must not keep
    // polling, exactly as the task list does not.
    if (watchers === 0) return;
    await refresh();
    schedule();
  }, POLL_MS);
}

export function useYtdl() {
  // Polling runs only while a view that wants it is mounted, matching how the
  // task list behaves: a hidden PWA must not keep asking.
  onMounted(() => {
    watchers += 1;
    void refresh();
    schedule();
  });
  onUnmounted(() => {
    watchers = Math.max(0, watchers - 1);
    if (watchers === 0 && timer) {
      clearTimeout(timer);
      timer = null;
    }
  });

  async function submit(url: string, mode: 'music' | 'music-video'): Promise<void> {
    await api.ytdlSubmit(url, mode);
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
    // Optimistic: the row goes now, and the next poll is the source of truth.
    downloads.value = downloads.value.filter((d) => d.requestId !== requestId);
    try {
      await api.ytdlDismiss(requestId);
    } finally {
      await refresh();
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
