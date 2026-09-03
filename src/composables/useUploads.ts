/**
 * Upload jobs, held OUTSIDE any component (spec 1022).
 *
 * The queue used to live inside UploadModal, which meant dismissing the modal
 * hid a transfer that was still running: no progress, no result, and no way to
 * retry a failure you never saw. Keeping the jobs at module scope lets the modal
 * start them and the Tasks list report them, so closing the sheet is a UI
 * choice rather than a loss of information.
 *
 * A job holds a live `File`, which cannot be serialised — so a page reload ends
 * any transfer in flight. That is a browser limit, not a design choice: there is
 * no background upload on iOS Safari, which is why the UI asks the user to keep
 * the app open rather than promising something it cannot deliver.
 */
import { computed, ref } from 'vue';
import { ApiError, api, type UploadResult } from '@/services/api';

export type UploadState = 'waiting' | 'sending' | 'done' | 'failed' | 'cancelled';

export interface UploadJob {
  id: number;
  file: File;
  name: string;
  kind: 'movie' | 'tv';
  title: string;
  season: string;
  state: UploadState;
  /** 0–1, driven by the request's own progress events. */
  progress: number;
  /** Total bytes, so the row can read like a download row rather than a bare bar. */
  size: number;
  /** Bytes per second, averaged over the transfer so far; 0 until it is moving. */
  speed: number;
  /** Bytes still to send, for the ETA. */
  remaining: number;
  /** The destination once done, or why it failed. */
  message: string;
  /**
   * The failure was a name collision, so replacing is a sensible offer. Kept
   * separate from `message` because it gates a DESTRUCTIVE action and must never
   * be inferred from display text.
   */
  replaceable: boolean;
}

// Module scope on purpose: one queue for the whole app, surviving every mount.
const jobs = ref<UploadJob[]>([]);
const cancels = new Map<number, () => void>();
let nextId = 1;
let draining = false;

function explain(e: unknown): string {
  if (!(e instanceof ApiError)) return 'Upload failed.';
  switch (e.code) {
    case 'file_exists':
      return 'A file of that name is already there — nothing was overwritten.';
    case 'destination_forbidden':
      return 'You do not have access to that folder.';
    case 'parent_unset':
      return 'No movie or TV folder is configured.';
    case 'cancelled':
      return 'Cancelled.';
    case 'offline':
      return 'Lost connection. The file was not fully uploaded.';
    case 'unreadable':
      return 'That file is no longer readable — it may have been moved or deleted.';
    default:
      return e.message || 'Upload failed.';
  }
}

/**
 * Confirm the file can still be read before sending a byte.
 *
 * A `File` is a handle to something on disk, and the user may have deleted or
 * moved it since picking it. Without this the read fails mid-transfer and the
 * request surfaces as a generic network error — telling the user to check their
 * connection when the real problem is a missing file.
 */
async function readable(file: File): Promise<boolean> {
  try {
    await file.slice(0, 1).arrayBuffer();
    return true;
  } catch {
    return false;
  }
}

async function runJob(job: UploadJob, overwrite: boolean): Promise<void> {
  job.state = 'sending';
  job.progress = 0;
  job.speed = 0;
  job.remaining = job.size;
  job.message = '';
  job.replaceable = false;
  const startedAt = Date.now();

  if (!(await readable(job.file))) {
    job.state = 'failed';
    job.message = explain(new ApiError('unreadable', 0));
    return;
  }

  const { promise, cancel } = api.uploadFile(
    { kind: job.kind, title: job.title, season: job.season, file: job.file, overwrite },
    (fraction) => {
      job.progress = fraction;
      const sent = fraction * job.size;
      job.remaining = Math.max(0, job.size - sent);
      // Averaged over the whole transfer rather than sampled between ticks: the
      // browser fires progress irregularly, and a per-tick rate reads as a
      // number that will not sit still.
      const elapsed = (Date.now() - startedAt) / 1000;
      job.speed = elapsed > 0.5 ? sent / elapsed : 0;
    },
  );
  cancels.set(job.id, cancel);
  try {
    const res: UploadResult = await promise;
    job.state = 'done';
    job.progress = 1;
    job.remaining = 0;
    job.speed = 0;
    job.message = res.destination;
  } catch (e) {
    job.state = e instanceof ApiError && e.code === 'cancelled' ? 'cancelled' : 'failed';
    job.message = explain(e);
    // Offered only for a real collision. An interrupted upload leaves a PARTIAL
    // file behind, and this is the only way to get past it.
    job.replaceable = e instanceof ApiError && e.code === 'file_exists';
  } finally {
    cancels.delete(job.id);
  }
}

/** One at a time: several large files at once would starve each other. */
async function drain(): Promise<void> {
  if (draining) return;
  draining = true;
  try {
    for (;;) {
      const next = jobs.value.find((j) => j.state === 'waiting');
      if (!next) break;
      await runJob(next, false);
    }
  } finally {
    draining = false;
  }
}

export function useUploads() {
  const active = computed(() =>
    jobs.value.filter((j) => j.state === 'sending' || j.state === 'waiting'),
  );
  const finished = computed(() =>
    jobs.value.filter((j) => j.state === 'done' || j.state === 'failed' || j.state === 'cancelled'),
  );

  function enqueue(
    files: File[],
    meta: { kind: 'movie' | 'tv'; title: string; season: string },
  ): UploadJob[] {
    const added = files.map((file) => ({
      id: nextId++,
      file,
      name: file.name,
      kind: meta.kind,
      title: meta.title,
      season: meta.season,
      state: 'waiting' as UploadState,
      progress: 0,
      size: file.size,
      speed: 0,
      remaining: file.size,
      message: '',
      replaceable: false,
    }));
    jobs.value = [...jobs.value, ...added];
    void drain();
    return added;
  }

  /** Retry a finished job. `overwrite` replaces a file already on the NAS. */
  function retry(id: number, overwrite = false): void {
    const job = jobs.value.find((j) => j.id === id);
    if (!job || job.state === 'sending' || job.state === 'waiting') return;
    if (overwrite) {
      void runJob(job, true);
      return;
    }
    job.state = 'waiting';
    void drain();
  }

  function cancel(id: number): void {
    cancels.get(id)?.();
  }

  /** Drop a finished job from the list; never touches one still running. */
  function dismiss(id: number): void {
    jobs.value = jobs.value.filter((j) => j.id !== id || j.state === 'sending' || j.state === 'waiting');
  }

  function clearFinished(): void {
    jobs.value = jobs.value.filter((j) => j.state === 'sending' || j.state === 'waiting');
  }

  return { jobs, active, finished, enqueue, retry, cancel, dismiss, clearFinished };
}
