#!/usr/bin/env bash
#
# Apply (and re-apply) SynoDL's protected-branch ruleset to main — the ONLY
# long-lived branch. Everything merges into main via a PR; the auto-merge
# workflow (.github/workflows/auto-merge.yml) refuses to schedule merges until
# this protection is in place, so run this once right after bootstrap.
#
# WHAT IT ENFORCES on main:
#   - Pull request required before merging (0 required approvals — we're a solo
#     maintainer and GitHub won't let you approve your own PR; raise this once
#     there are other maintainers).
#   - Required status checks (non-strict): the aggregate "CI gate" plus the
#     always-on roadmap + version guards (see REQUIRED_CHECKS). NON-strict on
#     purpose: requiring "up to date before merge" makes every merge invalidate
#     other in-flight PRs and forces a full re-run for an unrelated change. The
#     CI gate + release.yml's post-merge re-verify make that tax not worth it.
#   - Conversation resolution required.
#   - Force-pushes and branch deletion blocked.
#   - enforce_admins: rules apply to admins too (no bypass).
#   - Linear history NOT required (PRs land as merge commits).
#
# It also flips three REPO-LEVEL settings: allow_auto_merge (so the Auto-merge
# workflow can schedule every green PR), allow_merge_commit, and
# delete_branch_on_merge (auto-delete a PR's head branch once it merges;
# protected main is exempt via allow_deletions:false).
#
# PREREQUISITES:
#   - An authenticated GitHub CLI: `gh auth status` must succeed, with a token
#     that has admin rights on the repo.
#   - Branch protection on a PRIVATE repo requires a paid GitHub plan. It is
#     FREE once the repo is public; a free private repo gets a 403 here.
#
# USAGE:
#   scripts/setup-branch-protection.sh                  # defaults to zuptalo/synodl
#   REPO=owner/name scripts/setup-branch-protection.sh  # another repo
#   DRY_RUN=1 scripts/setup-branch-protection.sh        # print payloads, change nothing
#
# This is idempotent: the protection endpoint is a PUT, so re-running just
# restates the desired config.
set -euo pipefail

REPO="${REPO:-zuptalo/synodl}"
BRANCHES=(main)

# Required status check contexts.
#
# The heavy build/test/e2e jobs (the `verify` caller of build-test.yml) are
# CONDITIONALLY SKIPPED for doc/spec/tooling-only changes (see the `changes` job
# in ci.yml). A skipped check that is *required* would block the PR forever, so
# we must NOT require the individual "verify / *" contexts. Instead we require
# "CI gate" — an always-running aggregate that passes when every upstream job
# succeeded or was intentionally skipped, and fails if any actually failed.
#
# "Roadmap up to date" and "Version guard" are top-level ci.yml jobs that always
# run (cheap), so they are required directly too. The version guard passes on
# unchanged versions and only blocks downgrades / reuse of shipped versions.
#
# IMPORTANT: run this script only AFTER the ci.yml that defines "CI gate" is on
# main, or PRs will require a check that doesn't exist yet.
REQUIRED_CHECKS=(
  "CI gate"
  "Roadmap up to date"
  "Version guard"
)

if ! command -v gh >/dev/null 2>&1; then
  echo "error: gh (GitHub CLI) not found on PATH." >&2
  exit 1
fi
if ! gh auth status >/dev/null 2>&1; then
  echo "error: gh is not authenticated. Run 'gh auth login' first." >&2
  exit 1
fi

# Build the required_status_checks.checks array from REQUIRED_CHECKS.
checks_json=$(printf '%s\n' "${REQUIRED_CHECKS[@]}" \
  | jq -R '{context: .}' | jq -s '.')

payload=$(jq -n --argjson checks "$checks_json" '{
  required_status_checks: { strict: false, checks: $checks },
  enforce_admins: true,
  required_pull_request_reviews: {
    required_approving_review_count: 0,
    dismiss_stale_reviews: true,
    require_code_owner_reviews: false
  },
  restrictions: null,
  required_conversation_resolution: true,
  required_linear_history: false,
  allow_force_pushes: false,
  allow_deletions: false
}')

for branch in "${BRANCHES[@]}"; do
  echo "==> ${REPO}@${branch}"
  if [[ "${DRY_RUN:-}" == "1" ]]; then
    echo "$payload" | jq .
    continue
  fi
  echo "$payload" | gh api \
    --method PUT \
    -H "Accept: application/vnd.github+json" \
    "repos/${REPO}/branches/${branch}/protection" \
    --input - >/dev/null
  echo "    protection applied."
done

# Repo-level merge settings the auto-merge flow + housekeeping depend on:
#   - allow_auto_merge: lets the Auto-merge workflow schedule every green PR to
#     merge itself once required checks pass.
#   - allow_merge_commit: PRs land as MERGE COMMITS (release.yml verifies the
#     merge commit; feature history stays joined into main).
#   - delete_branch_on_merge: auto-delete a PR's head branch once it merges, so
#     stale feature branches don't pile up. SAFE here: main is protected with
#     allow_deletions:false, so it can never be auto-deleted.
echo "==> ${REPO} repo settings (auto-merge, merge commits, branch cleanup)"
if [[ "${DRY_RUN:-}" == "1" ]]; then
  echo '  { "allow_auto_merge": true, "allow_merge_commit": true, "delete_branch_on_merge": true }'
else
  gh api --method PATCH \
    -H "Accept: application/vnd.github+json" \
    "repos/${REPO}" \
    -F allow_auto_merge=true \
    -F allow_merge_commit=true \
    -F delete_branch_on_merge=true >/dev/null
  echo "    auto-merge + auto branch cleanup enabled."
fi

echo "Done. Verify in Settings -> Branches, or:"
echo "  gh api repos/${REPO}/branches/main/protection | jq ."
