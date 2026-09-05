# Feature Specification: The version you downloaded is not the one marked

**Feature Branch**: `fix/2015-recovered-version-reads`

**Created**: 2026-09-05

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: User description: "browse the titles and see if the downloaded titles and the downloaded version are marked correctly"

## Overview

Spec 1028 added a way to recover WHICH version a past download was, for
downloads recorded before SynoDL stored that alongside them. It reads the name
the file was downloaded under and takes the resolution plus "the token before
the site's trailing brand tag" as the encoder.

That last rule is wrong on the names these sites actually publish. Verified
against every recorded download on the reporting instance, the token it picks is
a subtitle or dubbing marker in every case:

```
The.Sheep.Detectives.2026.1080p.BluRay.x264.DD5.1.Pahe.SoftSub.ZarFilm.mkv
                                             ^^^^ encoder   ^^^^^^^ picked
Mutiny.2026.1080p.FHD.WEB-DL.Dubbed.ZarFilm.mkv
                             ^^^^^^ picked, and there is no encoder in this name
```

So the recovery either identifies the wrong encoder or invents one where the
name carries none.

**A second fault sits behind it.** Even with the right encoder, a download is
matched to a title by comparing its destination folder to the folder found on
the NAS, as exact text. Those two are routinely not the same string: a media
server renames the folder after the download lands.

```
sent to        movie/The Sheep Detectives
now on the NAS movie/The Sheep Detectives (2026)
```

Every send whose folder has since been renamed therefore matches nothing. On the
reporting instance, browsing 17 owned titles marked a version on 2 — and both of
those matched on file-name tokens, not on a send record. Not one send record
matched its own download.

Together these are what a user sees as "SynoDL knows I have this film but not
which copy", on exactly the downloads it sent itself.

## User Scenarios & Testing

### User Story 1 - The version I downloaded is the one marked (Priority: P1)

A user opens a title SynoDL downloaded for them before it recorded versions.
The option they actually have is marked, and the others are not.

**Acceptance**:
1. Given a recorded name carrying an encoder before a subtitle marker and a site
   brand, when the title is opened, then the option for that encoder is marked
   and no other option is.
2. Given a recorded name carrying no encoder at all, when the title is opened,
   then no option is marked.
3. Given the folder has since been renamed by a media server to add the release
   year, when the title is opened, then the recorded download still matches it.

### Edge Cases

- A scene-style name ending in `-GROUP` with nothing else after the resolution:
  the hyphen is what says it is a group rather than a brand, so it is recovered.
- A name whose only token after the resolution is the site's own brand: nothing
  is recovered. It cannot be told apart from an encoder, and guessing would mark
  a version the user may not have.
- A name with no resolution: unchanged — nothing is recovered.

## Requirements

### Functional Requirements

- **FR-001**: The encoder MUST be read from the tokens that follow the
  resolution, never from a fixed position in the name.
- **FR-002**: Release vocabulary that is not an encoder — subtitle and dubbing
  markers, video and audio codecs, source tags, channel counts, bare numbers —
  MUST NOT be recovered as one.
- **FR-003**: Where two or more candidates follow the resolution, the LAST is
  the site's own brand and the one before it is the encoder.
- **FR-004**: Where exactly one candidate follows the resolution, it MUST be
  recovered only if the name separated it scene-style with a hyphen. Otherwise
  it cannot be told apart from the site's brand, and marking the wrong version
  is worse than marking none (spec 1025).
- **FR-005**: Where nothing can be recovered, no option may be marked from this
  path. Absence of a mark remains not a claim that the user lacks the version.
- **FR-006**: A recorded download MUST be matched to a title folder by the same
  name comparison the library index already uses, not by exact text, so a folder
  renamed after the download still matches the record.
- **FR-007**: Where both the recorded folder and the folder on the NAS carry a
  release year, the years MUST agree — the same conservative rule the index
  applies, so a rename never matches the wrong title.
- **FR-008**: A season folder under a renamed series folder MUST still resolve to
  its season, so a series marks per season as before.

## Success Criteria

- **SC-001**: For a name of the form `<title>.<year>.<res>.<source>.<codec>.<encoder>.<submarker>.<brand>`, the recovered encoder is the encoder.
- **SC-002**: No name in the reporting instance's recorded history recovers a
  subtitle or dubbing marker as an encoder.
- **SC-003**: No option is marked for a name that carries no encoder.
- **SC-004**: A send whose folder has since gained a "(year)" suffix marks its
  version.

## Credential-Safety Impact

None. This changes how an already-stored name is parsed. No new data is read,
stored, or logged; the name still never leaves the server.
