# Feature Specification: YouTube workers can use an external PO-token provider

**Feature Branch**: `fix/2042-youtube-po-token-provider`

**Created**: 2026-10-03

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Production workers successfully received the saved YouTube cookies,
but YouTube still answered with its bot check. The home cluster already runs a
bgutil proof-of-origin token provider beside a current yt-dlp installation.

## Overview

An operator may point SynoDL at an internal bgutil HTTP provider. Download and
enumeration workers then use the `mweb` client, ask the installed provider plugin
for PO tokens, and enable Node for current YouTube JavaScript challenges. The
feature is opt-in because ordinary yt-dlp images do not necessarily contain the
plugin or Node.js.

## Requirements

- **FR-001**: An unset provider URL MUST leave existing worker commands unchanged.
- **FR-002**: A configured provider MUST add Node, `mweb`, and the bgutil HTTP
  provider URL to both download and enumeration workers as discrete argv values.
- **FR-003**: Saved cookies MUST continue to reach the worker through the existing
  one-use grant and file, never through the Job specification.
- **FR-004**: The provider endpoint MUST be operator configuration, never request
  input, and the deployment documentation MUST state that the selected pinned
  worker image needs the matching plugin and Node.js.
- **FR-005**: The home deployment MUST use an internal-only provider service and
  a worker image pinned by digest.

## Out of scope

- Guaranteeing that a PO token bypasses an already-blocked IP or account.
- Exposing the provider outside the cluster.
- Bundling or operating a provider for every SynoDL installation.
