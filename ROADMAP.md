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
| [0009](specs/0009-two-addresses-per/spec.md) | Two addresses per source, each with its own sign-in | 🔵 in-review |
| [0010](specs/0010-mark-version-you/spec.md) | Mark the version you downloaded, from what we sent | 🔵 in-review |
| [0011](specs/0011-keep-knowing-what-nas/spec.md) | Keep knowing what is on the NAS | 🔵 in-review |

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
| [1025](specs/1025-mark-version-you/spec.md) | Mark the version you actually downloaded | 🔵 in-review |
| [1026](specs/1026-tell-one-release/spec.md) | Tell one release from another by the file it makes | 🔵 in-review |
| [1027](specs/1027-choosing-quality-deliberate-act/spec.md) | Choosing a quality is a deliberate act | 🔵 in-review |
| [1028](specs/1028-recover-past-versions/spec.md) | Recover the version of downloads made before we recorded it | 🔵 in-review |
| [1029](specs/1029-forget-content-left-nas/spec.md) | Forget content that has left the NAS | 🔵 in-review |

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
| [2008](specs/2008-posters-and-owned/spec.md) | Posters and owned markers survive a source outage | 🔵 in-review |
| [2009](specs/2009-say-when-source/spec.md) | Say when a source session is not valid where it is being asked | 🔵 in-review |
| [2010](specs/2010-never-strand-app/spec.md) | Never strand the app on a source that is down | 🔵 in-review |
| [2011](specs/2011-source-omits-resolution/spec.md) | A source that omits the resolution can still be matched | 🔵 in-review |
| [2012](specs/2012-migration-never-ran/spec.md) | A migration added in the middle never runs | 🔵 in-review |
| [2013](specs/2013-orphaned-alt-credentials/spec.md) | Removing an address removes its credentials | 🔵 in-review |
| [2015](specs/2015-recovered-version-reads/spec.md) | The version you downloaded is not the one marked | 🟢 shipped |
| [2016](specs/2016-data-dir-special-chars/spec.md) | A DATA_DIR with punctuation in it opens the wrong database | 🟢 shipped |
