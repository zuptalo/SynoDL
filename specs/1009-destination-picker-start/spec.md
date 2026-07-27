# Feature Specification: Destination picker starts in the selected folder + task-level new folder

**Feature Branch**: `feat/1009-destination-picker-start`

**Created**: 2026-07-28

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Operator report + request: "When I pick a favorite destination and then tap it to browse or
create a folder, browsing still starts at the root instead of the selected folder. Also let me create a
folder inside the selected folder right from the task screen — one button that creates it and sets it as
the destination."

## Functional Requirements

- **FR-001** Opening the destination picker MUST start browsing inside the currently-selected destination
  (e.g. a picked favorite), not at the share root, so drilling/creating continues from there.
- **FR-002** The new-task screen MUST offer a "New folder in <destination>" action (shown when a
  destination is selected) that creates a subfolder inside it and sets it as the task's destination,
  without opening the picker. It reuses the ACL-gated CreateFolder endpoint.

## Testing

E2E: after selecting a folder, reopening the picker starts at that folder's path; the task-level new
folder creates and selects `<dest>/<name>`.
