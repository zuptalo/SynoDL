<!--
RELEASE PR — any PR into main that bumps package.json's version.

Use this template when the point of the PR is to cut a release (select it with
?template=release.md in the compose URL). Merging a PR whose version bump is
new SHIPS that release: release.yml re-verifies the merge commit, publishes the
image tags (latest, main-<sha>, X.Y.Z, X.Y), tags vX.Y.Z, and cuts a GitHub
release.

Bump with `npm run release:patch` (or :minor / :major) and commit the result —
the CI "Version guard" blocks downgrades and reuse of already-shipped versions.

The "## Changes" bullets below are what reviewers see is shipping. The GitHub
release notes are generated automatically from the Conventional-Commit subjects
since the last tag, so keeping commit subjects clean keeps the release notes
clean.

Auto-merge is enabled automatically: once the required checks are green, GitHub
merges the PR on its own. Keep it a draft to hold it.
-->

## Release vX.Y.Z

<!-- Replace X.Y.Z with the bumped package.json version. -->

- [ ] `package.json` version bumped via `npm run release:{patch|minor|major}` to a new, unreleased version.

## Changes

<!-- One user-facing one-liner per change shipping in this release. -->

- 

## Notes / upgrade considerations

<!-- Anything operators should know (config, breaking changes). Delete if none. -->
