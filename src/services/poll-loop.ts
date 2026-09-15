/**
 * A repeating poll that can actually be stopped.
 *
 * This exists because the obvious version cannot be, and reads as though it can
 * (spec 2030). Both live lists kept their own copy of it, and both were wrong
 * the same way:
 *
 *     const tick = async () => {
 *       pollTimer = null;                    // the handle stop() looks for
 *       if (visible) await refresh();        // ← stop() lands in here
 *       pollTimer = setTimeout(tick, MS);    // ...and the tick re-arms anyway
 *     };
 *     function stopPolling() { if (pollTimer) { clearTimeout(pollTimer); ... } }
 *
 * A stop arriving while the poll's own request was in flight found a null handle,
 * did nothing, and the tick re-armed behind it. The loop was then immortal. In
 * production that meant the five-second fallback running alongside a perfectly
 * healthy stream: 1577 requests in 2h12m, 157 of them during one 790-second open
 * stream, asking the NAS for a list that was already being pushed.
 *
 * The fix is that whether to continue is held in explicit state rather than
 * inferred from a timer handle. A handle says "a tick is scheduled", which is
 * not the same question and is false for the whole duration of a tick — exactly
 * the window in which a live stream calls stop().
 *
 * Shared rather than fixed twice: this is the kind of code that reads as correct
 * in both copies and is wrong in both, and one module is the only way the unit
 * test covers what actually runs.
 */
export interface PollLoop {
  /** Begin polling. A no-op if already running, so two callers cannot double it. */
  start(): void;
  /** Stop polling, including from inside an in-flight tick. */
  stop(): void;
}

export function createPollLoop(run: () => Promise<void> | void, intervalMs: number): PollLoop {
  let timer: ReturnType<typeof setTimeout> | null = null;
  let running = false;

  const tick = async () => {
    timer = null;
    // Re-checked here as well as after the await: stop() may have arrived
    // between the timer firing and this line.
    if (!running) return;
    await run();
    // THE LINE THAT MATTERS. The old loop re-armed unconditionally, so a stop
    // issued during `run()` was undone the moment it resolved.
    if (running) timer = setTimeout(tick, intervalMs);
  };

  return {
    start(): void {
      if (running) return;
      running = true;
      timer = setTimeout(tick, intervalMs);
    },
    stop(): void {
      running = false;
      if (timer) {
        clearTimeout(timer);
        timer = null;
      }
    },
  };
}
