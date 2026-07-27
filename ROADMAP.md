<!-- GENERATED FILE — do not edit by hand.
     Regenerate with: make roadmap   (or python3 scripts/roadmap-gen.py)
     Source of truth: specs/<NNNN-slug>/spec.md (Status line + directory number).
     CI fails if this file is out of date. -->

# SynoDL Roadmap

Every change ships through a numbered spec (see [CONTRIBUTING.md](CONTRIBUTING.md)).
Specs are grouped by category band; status moves
`planned → in-progress → in-review → shipped`.

## 📌 Planned Features (0001–0999)

| Spec | Title | Status |
|------|-------|--------|
| [0001](specs/0001-connect-tasks-mvp/spec.md) | Connect to Download Station, view tasks, and add downloads | 🟢 shipped |
| [0002](specs/0002-live-task-updates/spec.md) | Live task updates, task detail view, and download failure reasons | 🔵 in-review |
| [0003](specs/0003-stateful-multi-user/spec.md) | Stateful multi-user rework — setup wizard, SynoDL accounts, folder access, and Web Push | 🟢 shipped |
| [0004](specs/0004-task-list-bulk/spec.md) | Task-list bulk actions, selection mode, app badge, and newest-first sort | 🟢 shipped |

## ⚡ Ad-hoc (1001–1999)

| Spec | Title | Status |
|------|-------|--------|
| [1001](specs/1001-dsm-auth-errors/spec.md) | Recognize every DSM 7 sign-in failure with its own message | 🟢 shipped |
| [1002](specs/1002-admin-settings/spec.md) | Admin settings — edit & test the NAS connection, change password, themes, and glass UI | 🔵 in-review |
| [1003](specs/1003-update-page/spec.md) | In-app update page — what's new, one-tap OK, and self-healing apply | 🔵 in-review |
| [1004](specs/1004-notification-prefs/spec.md) | Per-user notification preferences with task attribution | 🔵 in-review |
| [1005](specs/1005-bulk-urls/spec.md) | Bulk-paste URLs with mixed delimiters, added in batches | 🔵 in-review |
| [1006](specs/1006-destinations/spec.md) | Destination overhaul — cancel, default, favorites, and subfolder creation | 🔵 in-review |
| [1007](specs/1007-copy-redownload/spec.md) | Copy the download link and re-download from the detail view | 🔵 in-review |
| [1008](specs/1008-install-guard/spec.md) | PWA install guard — sign in only from the installed app | 🔵 in-review |
| [1009](specs/1009-destination-picker-start/spec.md) | Destination picker starts in the selected folder + task-level new folder | 🔵 in-review |
| [1011](specs/1011-destination-prefs-server/spec.md) | Per-user destination preferences on the server, with self-cleaning | 🔵 in-review |

## 🐛 Hotfixes & Bug Fixes (2001+)

| Spec | Title | Status |
|------|-------|--------|
| [2001](specs/2001-new-task-modal/spec.md) | New-task modal shows a false "Could not reach the server" after the task is created | 🟢 shipped |
