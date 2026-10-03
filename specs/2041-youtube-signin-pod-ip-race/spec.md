# Feature Specification: YouTube workers wait for their pod address before fetching the saved sign-in

**Feature Branch**: `fix/2041-youtube-signin-pod-ip-race`

**Created**: 2026-10-03

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: Production YouTube retries fetched the newly saved cookie only
intermittently. Server logs showed `signin fetch denied reason=address` and the
workers then ran anonymously.

## Overview

A worker's init container can reach SynoDL before Kubernetes has published that
pod's `status.podIP`. The sign-in endpoint correctly refuses the request because
it cannot yet prove that the caller owns the job-bound grant, but the init
container previously made only one request. It then deliberately failed open
with an empty cookie file, causing yt-dlp to run anonymously.

The grant is not consumed by an address refusal. The init container can safely
retry it for a short, bounded period until Kubernetes publishes the pod IP.

## Requirements

- **FR-001**: A sign-in init container MUST retry an unredeemed grant for a
  bounded period before falling back to an empty cookie file.
- **FR-002**: A failed address check MUST NOT consume the grant; the same pod can
  redeem it after Kubernetes publishes its IP.
- **FR-003**: Address-check logs MUST distinguish an unpublished pod/IP, a
  Kubernetes API failure, an invalid caller address, and a genuine mismatch,
  without logging addresses, grants, or cookies.
- **FR-004**: The regression MUST be covered by tests for the bounded worker
  retry and for a grant succeeding after the pod IP becomes visible.

## Out of scope

- Making an expired or YouTube-invalidated browser session valid again.
- Bypassing YouTube rate limits, IP blocks, or PO-token requirements.
