# Tasks: YouTube workers can use an external PO-token provider

**Input**: [spec.md](./spec.md)

**Tests**: REQUIRED.

- [x] T001 Confirm the existing provider, plugin, Node runtime, and exact yt-dlp arguments live.
- [x] T002 Expose the provider through an internal-only ClusterIP service and verify cross-namespace reachability.
- [x] T003 Add opt-in provider configuration and pass it to download and enumeration Jobs.
- [x] T004 Add regression tests for the disabled and enabled command shapes.
- [x] T005 Run the full server gates and build a local production image.
- [ ] T006 Deploy locally to home k3s with a digest-pinned compatible worker image; verify the provider smoke test and a real retry.
- [ ] T007 Merge, publish `:latest`, restore the production deployment, and mark shipped.
