# Feature Specification: ZarFilm is reached only at the address the operator gives

**Feature Branch**: `feat/1045-zarfilm-uses-operator`

**Created**: 2026-09-27

**Status**: in-progress
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "https://zarfilm.com seems to be going down a lot which makes it an unreliable source, let's drop the support for https://zarfilm.com completly and just expect user to provide the URL and authentication for the current FQDN of the website instead, right now that custom FQDN is pointing at https://zhomis.info"

## Overview

The ZarFilm driver shipped with `zarfilm.com` compiled in (spec 0007), later
overridable by an operator's main address (0009) and backed by a mirror (1020,
pre-filled with `https://zhomis.info`). The built-in domain now goes down often
enough that a source relying on it is unreliable, and the site keeps moving —
so an address in the release is wrong on the site's schedule, not ours.

This drops the built-in address entirely. The operator supplies the site's
current address and the sign-in material issued by it.

It also fixes two faults the built-in address was hiding:

1. **Verification ignored the configured address.** Save-time verification and
   the keep-alive probe always checked `zarfilm.com`. With it down, a source
   working on its mirror verified as unreachable and, after four probes, was
   marked "needs signing in again".
2. **An empty main address became the mirror.** The server validated the main
   address with the same function as the alternate, which fills an empty value
   with the driver's default mirror — so the form's "leave blank for built-in"
   quietly stored zhomis.info as the main address.

### Amends earlier specs

- 0007 FR-017 ("support zarfilm.com"), research §posters: the site is supported
  at the operator's address; `zarfilm.com` is no longer contacted.
- 0009 FR-002 / US1 AS2 ("neither given → built-in address"): does not hold for
  a driver with no built-in address; such a driver requires one.
- 1020 FR-002 (driver-declared default mirror): ZarFilm declares none.

## Requirements

- **FR-001**: The ZarFilm driver MUST NOT contact, accept links on, or proxy
  images from `zarfilm.com` (or any host) unless it is configured as the
  source's address. Its only fixed host is the signed-download storage domain.
- **FR-002**: A driver MAY declare that it has no address of its own; for such a
  driver a source MUST NOT be saved without a main address (`address_required`),
  and the add-source form MUST ask for the "Site address" as required.
- **FR-003**: An alternate address given without a main one MUST be taken as the
  main address. A source stored that way before this change MUST keep working,
  using that address and its own sign-in material as the main ones.
- **FR-004**: Verification (save-time and keep-alive) MUST check the configured
  addresses, in the same order and with the same per-address material as page
  fetches.
- **FR-005**: An empty main address MUST NOT be filled with a driver's default
  mirror.
- **FR-006**: The older single-source route, which has no address fields, MUST
  refuse such a driver.
- **FR-007**: Dev/e2e builds pointed at the in-repo fake site need no address
  (the fake stands in for it); release builds cannot do this.

## Operational note

No ZarFilm source is configured on the home cluster, so nothing there needs
migrating; the operator adds one with `https://zhomis.info` and a sign-in
captured there.
