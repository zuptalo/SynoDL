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
| [0009](specs/0009-two-addresses-per/spec.md) | Two addresses per source, each with its own sign-in | 🟢 shipped |
| [0010](specs/0010-mark-version-you/spec.md) | Mark the version you downloaded, from what we sent | 🟢 shipped |
| [0011](specs/0011-keep-knowing-what-nas/spec.md) | Keep knowing what is on the NAS | 🟢 shipped |
| [0012](specs/0012-download-youtube-audio/spec.md) | Save YouTube music and music videos to the library | 🟢 shipped |
| [0013](specs/0013-youtube-downloads-managed/spec.md) | YouTube downloads you can watch, keep, and retry | 🟢 shipped |

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
| [1023](specs/1023-zarfilm-titles-carry/spec.md) | ZarFilm titles carry an IMDb link and a synopsis | 🟢 shipped |
| [1024](specs/1024-filter-and-sort/spec.md) | Filter and sort every source the same way | 🟢 shipped |
| [1025](specs/1025-mark-version-you/spec.md) | Mark the version you actually downloaded | 🟢 shipped |
| [1026](specs/1026-tell-one-release/spec.md) | Tell one release from another by the file it makes | 🟢 shipped |
| [1027](specs/1027-choosing-quality-deliberate-act/spec.md) | Choosing a quality is a deliberate act | 🟢 shipped |
| [1028](specs/1028-recover-past-versions/spec.md) | Recover the version of downloads made before we recorded it | 🟢 shipped |
| [1029](specs/1029-forget-content-left-nas/spec.md) | Forget content that has left the NAS | 🟢 shipped |
| [1030](specs/1030-discover-opens-newest-titles/spec.md) | Discover opens on the newest titles | 🟢 shipped |
| [1031](specs/1031-schema-drift-guard/spec.md) | Notice when a database's schema is not what it should be | 🟢 shipped |
| [1032](specs/1032-show-release-year/spec.md) | Show release year and readable genres on titles | 🟢 shipped |
| [1033](specs/1033-give-youtube-downloads/spec.md) | Give YouTube downloads their own button | 🟢 shipped |
| [1034](specs/1034-make-youtube-downloads/spec.md) | Make YouTube downloads readable in the task list | 🟢 shipped |
| [1035](specs/1035-card-captions-read/spec.md) | Card captions read like the detail sheet | 🟢 shipped |
| [1036](specs/1036-telegram-intake-architect-reports/spec.md) | Telegram intake architect reports | ⚪ planned |
| [1037](specs/1037-item-rows-and-polish/spec.md) | A track inside a playlist looks like any other download | 🔵 in-review |
| [1038](specs/1038-ytdl-live-updates/spec.md) | YouTube downloads update as they happen | ⚪ planned |

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
| [2008](specs/2008-posters-and-owned/spec.md) | Posters and owned markers survive a source outage | 🟢 shipped |
| [2009](specs/2009-say-when-source/spec.md) | Say when a source session is not valid where it is being asked | 🟢 shipped |
| [2010](specs/2010-never-strand-app/spec.md) | Never strand the app on a source that is down | 🟢 shipped |
| [2011](specs/2011-source-omits-resolution/spec.md) | A source that omits the resolution can still be matched | 🟢 shipped |
| [2012](specs/2012-migration-never-ran/spec.md) | A migration added in the middle never runs | 🟢 shipped |
| [2013](specs/2013-orphaned-alt-credentials/spec.md) | Removing an address removes its credentials | 🟢 shipped |
| [2015](specs/2015-recovered-version-reads/spec.md) | The version you downloaded is not the one marked | 🟢 shipped |
| [2016](specs/2016-data-dir-special-chars/spec.md) | A DATA_DIR with punctuation in it opens the wrong database | 🟢 shipped |
| [2017](specs/2017-hide-owned-migration-skipped/spec.md) | "Hide what I have" never saved, and every view save answered 500 | 🟢 shipped |
| [2019](specs/2019-orphaned-rows-sweep/spec.md) | Remove rows the cascades should have taken with them | 🟢 shipped |
| [2020](specs/2020-download-without-lyrics/spec.md) | A download without lyrics is not a failure | 🟢 shipped |
| [2021](specs/2021-ytdl-flag-arity/spec.md) | Progress reporting and the traversal guard, actually switched on | 🔵 in-review |
| [2022](specs/2022-expand-job-identity/spec.md) | A group's enumeration job is not the group finishing | 🔵 in-review |
| [2023](specs/2023-podlog-accept/spec.md) | Reading a worker's output, actually permitted by the API | 🔵 in-review |
| [2024](specs/2024-group-sheet-refetch/spec.md) | A group's items stop reloading on every poll | 🔵 in-review |
| [2025](specs/2025-item-detail-lookup/spec.md) | An item's details, actually reachable | 🔵 in-review |
| [2026](specs/2026-steady-row-height/spec.md) | A row that stays the height it was | 🔵 in-review |
