/**
 * Pure decision logic for the in-app update flow (spec 1003). Given whether the
 * service worker has a waiting update, the running bundle's version, and any
 * "pending" target recorded when the user last pressed OK, decide what to do.
 *
 * The recorded pending version is what makes an interrupted update self-heal:
 * if the user pressed OK but the app was closed before the reload finished, the
 * next launch still has a waiting worker AND a pending target it hasn't reached,
 * so we apply it automatically instead of prompting again.
 */
export interface UpdateDecision {
  /** The update finished (we're now running the target) — drop the pending flag. */
  clearFlag: boolean;
  /** Finish an interrupted update now, without prompting. */
  autoApply: boolean;
  /** Show the update page and let the user press OK. */
  prompt: boolean;
}

export function decideUpdate(opts: {
  waiting: boolean;
  runningVersion: string;
  pendingVersion: string | null;
}): UpdateDecision {
  const { waiting, runningVersion, pendingVersion } = opts;
  const none: UpdateDecision = { clearFlag: false, autoApply: false, prompt: false };

  // We're now running the version we were updating to → the update completed.
  if (pendingVersion && pendingVersion === runningVersion) {
    return { clearFlag: true, autoApply: false, prompt: false };
  }
  if (!waiting) return none;
  // A waiting worker plus a target we haven't reached = an apply that was
  // interrupted last session; finish it silently.
  if (pendingVersion && pendingVersion !== runningVersion) {
    return { clearFlag: false, autoApply: true, prompt: false };
  }
  // First time we've seen this update → ask the user.
  return { clearFlag: false, autoApply: false, prompt: true };
}
