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
| [0002](specs/0002-live-task-updates/spec.md) | Live task updates, task detail view, and download failure reasons | 🟢 shipped |
| [0003](specs/0003-stateful-multi-user/spec.md) | Stateful multi-user rework — setup wizard, SynoDL accounts, folder access, and Web Push | 🟢 shipped |
| [0004](specs/0004-task-list-bulk/spec.md) | Task-list bulk actions, selection mode, app badge, and newest-first sort | 🟢 shipped |
| [0005](specs/0005-source-catalog/spec.md) | Download-source catalog — browse, search, and send an admin-configured provider to Download Station | 🟢 shipped |
| [0006](specs/0006-per-user-download/spec.md) | Per-User Download Statistics and Richer Notifications | 🟢 shipped |
| [0007](specs/0007-multiple-download-sources/spec.md) | Multiple Download Sources | 🟢 shipped |
| [0008](specs/0008-show-which-discover/spec.md) | Show which Discover titles you already have | 🟢 shipped |

## ⚡ Ad-hoc (1001–1999)

| Spec | Title | Status |
|------|-------|--------|
| [1001](specs/1001-dsm-auth-errors/spec.md) | Recognize every DSM 7 sign-in failure with its own message | 🟢 shipped |
| [1002](specs/1002-admin-settings/spec.md) | Admin settings — edit & test the NAS connection, change password, themes, and glass UI | 🟢 shipped |
| [1003](specs/1003-update-page/spec.md) | In-app update page — what's new, one-tap OK, and self-healing apply | 🟢 shipped |
| [1004](specs/1004-notification-prefs/spec.md) | Per-user notification preferences with task attribution | 🟢 shipped |
| [1005](specs/1005-bulk-urls/spec.md) | Bulk-paste URLs with mixed delimiters, added in batches | 🟢 shipped |
| [1006](specs/1006-destinations/spec.md) | Destination overhaul — cancel, default, favorites, and subfolder creation | 🟢 shipped |
| [1007](specs/1007-copy-redownload/spec.md) | Copy the download link and re-download from the detail view | 🟢 shipped |
| [1008](specs/1008-install-guard/spec.md) | PWA install guard — sign in only from the installed app | 🟢 shipped |
| [1009](specs/1009-destination-picker-start/spec.md) | Destination picker starts in the selected folder + task-level new folder | 🟢 shipped |
| [1011](specs/1011-destination-prefs-server/spec.md) | Per-user destination preferences on the server, with self-cleaning | 🟢 shipped |
| [1013](specs/1013-tasks-search-notif/spec.md) | Foreground-aware notifications, Tasks search bar, fewer tabs | 🟢 shipped |
| [1014](specs/1014-discover-filter-sheet/spec.md) | Discover filter sheet polish | 🟢 shipped |
| [1015](specs/1015-discover-polish-batch/spec.md) | Discover polish batch | 🟢 shipped |
| [1016](specs/1016-tasks-view-poster/spec.md) | Tasks view — posters, cleaner titles, Open in Discover | 🟢 shipped |
| [1017](specs/1017-download-statistics-readable/spec.md) | Download statistics — readable history + totals | 🟢 shipped |
| [1018](specs/1018-discover-infinite-scroll/spec.md) | Discover keeps loading ahead of a fast scroller | 🟢 shipped |
| [1019](specs/1019-imdb-rating-links/spec.md) | The IMDb rating opens the title on IMDb | 🟢 shipped |
| [1020](specs/1020-fall-back-source/spec.md) | Alternate Domain Fallback for a Download Source | 🟢 shipped |
| [1021](specs/1021-name-new-downloads/spec.md) | Name new downloads the way Plex expects | 🟢 shipped |
| [1022](specs/1022-upload-file-straight/spec.md) | Upload a file straight into your library | 🟢 shipped |
| [1023](specs/1023-zarfilm-titles-carry/spec.md) | ZarFilm titles carry an IMDb link and a synopsis | 🔵 in-review |
| [1024](specs/1024-filter-and-sort/spec.md) | Filter and sort every source the same way | 🔵 in-review |

## 🐛 Hotfixes & Bug Fixes (2001+)

| Spec | Title | Status |
|------|-------|--------|
| [2001](specs/2001-new-task-modal/spec.md) | New-task modal shows a false "Could not reach the server" after the task is created | 🟢 shipped |
| [2002](specs/2002-discover-text-search/spec.md) | Fix Discover text search filtering | 🟢 shipped |
| [2003](specs/2003-statistics-filter-segments/spec.md) | Statistics filter segments sit in cards | 🟢 shipped |
| [2004](specs/2004-series-download-options/spec.md) | Order series download options by season then size | 🟢 shipped |
| [2005](specs/2005-season-groups-download/spec.md) | Obvious season dividers in the download options list | 🟢 shipped |
| [2006](specs/2006-release-year-sort/spec.md) | Release-year sort no longer leads with year-less titles | 🟢 shipped |
| [2007](specs/2007-release-year-sort/spec.md) | Release-year sort is fast again, and Discover opens on Most popular | 🟢 shipped |
