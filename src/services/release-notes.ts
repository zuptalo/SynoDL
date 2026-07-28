/**
 * "What's new" derivation for the update page.
 *
 * Every build bakes the Conventional-Commit changes since the last release tag
 * into BOTH the client (`__RELEASE_NOTES__`, the RUNNING build) and the server
 * (`/v1/config` releaseNotes, the INCOMING build). Because a home deploy ships on
 * every merge — not just version bumps — the incoming list keeps growing between
 * tags, so showing it whole repeats changes the user already has. Instead we show
 * the DELTA: incoming commits (by short SHA) that the running build doesn't have,
 * limited to user-facing types and stripped of the `type(scope):` prefix so each
 * line reads as plain-language release-note copy.
 */
export interface RawNote {
  sha: string;
  subject: string;
}

// Types that reach users' "What's new" (constitution Principle V). Everything
// else (chore/ci/build/docs/refactor/style/test/deps) is internal and dropped.
const USER_FACING = /^(feat|fix|perf|security)(\([^)]*\))?!?:\s*/i;

/**
 * The lines to show someone upgrading FROM `running` TO `incoming`.
 * @param incoming the new build's notes (from /v1/config)
 * @param running  the current build's notes (compile-time __RELEASE_NOTES__)
 */
export function whatsNew(incoming: RawNote[], running: RawNote[]): string[] {
  const have = new Set(running.map((n) => n.sha));
  const seen = new Set<string>();
  const out: string[] = [];
  for (const n of incoming) {
    if (have.has(n.sha) || seen.has(n.sha)) continue; // already running, or a dupe
    seen.add(n.sha);
    const m = USER_FACING.exec(n.subject);
    if (!m) continue; // internal change — not user-facing
    const line = n.subject.slice(m[0].length).trim();
    if (line) out.push(line);
  }
  return out;
}
