# Feature Specification: The filter sheet scrolls with a mouse wheel on desktop

**Feature Branch**: `fix/2036-filter-sheet-scrolls-desktop`

**Created**: 2026-09-27

**Status**: in-progress
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped.
     This line is the source of truth for the spec's row in ROADMAP.md;
     bump it as the work moves through the pipeline. The spec id and category
     are derived from the directory number (0001+ planned, 1001+ ad-hoc,
     2001+ hotfix), so do not restate them by hand. -->

**Input**: User description: "the sorting doesn't show fully on desktop, it doesn't scroll down to the accending and decending part, but looks fine on ios pwa installation"

## Overview

Both filter sheets (Tasks, and Discover's) are Ionic sheet modals opened part
way (85% / 75%). By default a sheet's content only scrolls once the sheet has
been dragged fully open. With a finger that happens naturally — swipe up, then
keep scrolling — which is why the installed iOS app was fine. With a mouse the
wheel neither drags the sheet nor scrolls it, and on a desktop window the sheet's
lower part sits below the edge: measured at 1280×720, "Descending" at y=891,
unreachable, with the status filters below it.

## Requirements

- **FR-001**: Both filter sheets MUST scroll their content at any height they
  are open to (`expand-to-scroll` off), so everything in them is reachable with
  a mouse wheel. Touch behaviour — dragging the sheet between heights — is
  unchanged.
