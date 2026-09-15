import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPollLoop } from './poll-loop';

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe('createPollLoop', () => {
  it('runs on the interval while started', async () => {
    let n = 0;
    const loop = createPollLoop(() => void n++, 1000);
    loop.start();
    await vi.advanceTimersByTimeAsync(3500);
    expect(n).toBe(3);
  });

  it('stops before the first tick', async () => {
    let n = 0;
    const loop = createPollLoop(() => void n++, 1000);
    loop.start();
    loop.stop();
    await vi.advanceTimersByTimeAsync(5000);
    expect(n).toBe(0);
  });

  /**
   * THE REGRESSION THIS EXISTS FOR.
   *
   * In production the fallback poll ran alongside a healthy stream — 157
   * requests during one 790-second open stream. The stream calls stop() the
   * moment it goes live, and that call landed while the poll's own request was
   * in flight. The old loop cleared its timer handle BEFORE awaiting, and stop()
   * only acted on a non-null handle, so the call did nothing and the tick
   * re-armed. The loop was then immortal.
   */
  it('stops when stop() lands while its own request is in flight', async () => {
    let n = 0;
    let release!: () => void;
    const inFlight = new Promise<void>((r) => (release = r));
    const loop = createPollLoop(async () => {
      n += 1;
      if (n === 1) await inFlight; // hold the first tick open
    }, 1000);

    loop.start();
    await vi.advanceTimersByTimeAsync(1000); // tick 1 starts and blocks
    expect(n).toBe(1);

    loop.stop(); // the stream went live mid-request
    release();
    await Promise.resolve();

    await vi.advanceTimersByTimeAsync(10_000);
    expect(n, 'the poll kept running after it was stopped').toBe(1);
  });

  it('resumes after a stop, so a dropped stream falls back again', async () => {
    let n = 0;
    const loop = createPollLoop(() => void n++, 1000);
    loop.start();
    await vi.advanceTimersByTimeAsync(1000);
    loop.stop();
    await vi.advanceTimersByTimeAsync(5000);
    expect(n).toBe(1);
    loop.start();
    await vi.advanceTimersByTimeAsync(2000);
    expect(n).toBe(3);
  });

  it('does not run two loops if started twice', async () => {
    let n = 0;
    const loop = createPollLoop(() => void n++, 1000);
    loop.start();
    loop.start();
    await vi.advanceTimersByTimeAsync(3000);
    expect(n).toBe(3);
  });
});
