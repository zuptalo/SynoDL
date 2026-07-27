/**
 * Task-list polling. The NAS is the source of truth (constitution Principle
 * IV); this composable polls /v1/tasks while its owner view is mounted AND the
 * tab is visible — a hidden PWA must not keep the NAS awake.
 */
import { onMounted, onUnmounted, ref } from 'vue';
import { api } from '@/services/api';
import type { Stats, Task } from '@/types/task';

const POLL_MS = 3000;

export function useTasks() {
  const tasks = ref<Task[]>([]);
  const stats = ref<Stats>({ downloadSpeed: 0, uploadSpeed: 0 });
  const loaded = ref(false);
  const error = ref('');
  let timer: ReturnType<typeof setTimeout> | null = null;
  let stopped = false;

  async function refresh(): Promise<void> {
    try {
      const res = await api.tasks();
      tasks.value = res.tasks;
      stats.value = res.stats;
      error.value = '';
    } catch (e) {
      // Session expiry is handled globally (api dispatches, router bounces);
      // everything else surfaces inline without clearing the last good list.
      error.value = e instanceof Error ? e.message : 'error';
    } finally {
      loaded.value = true;
    }
  }

  function schedule(): void {
    if (stopped) return;
    timer = setTimeout(async () => {
      if (document.visibilityState === 'visible') await refresh();
      schedule();
    }, POLL_MS);
  }

  function onVisibility(): void {
    // Coming back to the app: refresh immediately instead of waiting a tick.
    if (document.visibilityState === 'visible') void refresh();
  }

  onMounted(() => {
    void refresh();
    schedule();
    document.addEventListener('visibilitychange', onVisibility);
  });
  onUnmounted(() => {
    stopped = true;
    if (timer) clearTimeout(timer);
    document.removeEventListener('visibilitychange', onVisibility);
  });

  return { tasks, stats, loaded, error, refresh };
}
