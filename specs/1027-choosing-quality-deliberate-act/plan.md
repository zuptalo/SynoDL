# Implementation Plan: Choosing a quality is a deliberate act

**Spec**: [spec.md](./spec.md) · **Branch**: `feat/1027-choosing-quality-deliberate-act`

## Summary

Three defects on one screen, all found by driving the running app against a
seeded library rather than by reading the code:

1. An option is pre-picked on open. On a part-owned series that pick is
   **invisible** — season 1's option inside a collapsed season — while the user
   is looking at season 3, and the send button reads "Send 4 to NAS".
2. Sending a season the user does not have warns that they already have it.
3. A successful send raises a toast over a button that has just become a live
   status control for that same download.

## Research (running app, seeded library)

Two sources configured against the in-repo fakes, with `Zar Title 3` seeded as
seasons 1–2 present and season 3 missing. Read out of the live sheet:

```
selected:     "s1-0"          <- season 1, inside a COLLAPSED season
openSeason:   "3"
visible:      ["s3-4","s3-5"] <- the only options on screen
sendLabel:    "Send 4 to NAS"
```

So the primary action was armed with four episodes of a season already on disk,
chosen by nobody. Sending season 3 then produced:

> Download it again? You already have this.

which is exactly backwards — season 3 is the one season not present.

## Technical Context

**Language/Version**: TypeScript 5 / Vue 3 + Ionic
**Primary Dependencies**: none added
**Storage**: none
**Testing**: Playwright (this flow had **no** coverage at all); vitest
**Project Type**: web (Vue PWA + Go proxy)
**Constraints**: client-only; no API, allowlist, NAS or persistence change

## Constitution Check

| Principle | Verdict |
|---|---|
| I — Spec-first | Behaviour researched against the running app, then specified, then changed. |
| II — TDD | This flow had zero tests. Coverage is added with the change, and each test was watched failing against the old behaviour before it passed. |
| III — Credential boundary | Untouched — nothing crosses the boundary that did not before. |
| IV — Stateless where it can be | Selection is transient UI state, as before. |
| V — Release-note commit subjects | `fix(discover): don't pick a download for you, and only warn about real repeats` |
| VI — Ionic-first UI | Uses the existing `ion-radio-group` and `ion-accordion-group`; no new component. |

No violations.

## Design

### 1. Nothing is picked until picked

`selected` starts empty and the load path no longer chooses. The stored quality
preference is not discarded — it now decides which **tier tab** opens, so the
preference still lands the user where they want without making the choice.

### 2. A pick is always visible

The invisible pick is the actual defect, so the invariant is stated directly:
**a held pick must be on screen**. Two places can break it, and both now clear it:

- switching quality tier, when the pick belongs to another tier;
- opening another season, when the pick belongs to another season.

### 3. The next step comes into view

A watcher on the pick scrolls a small anchor placed between the option list and
what follows it — the episode selection for a season pack, the send button for a
movie. One anchor serves both because the episode list only renders for a pack.

### 4. Warn about what is being sent, not what is owned

```
season pack ─▶ is THAT season present?
otherwise   ─▶ is the title present?
```

A title still arriving warns either way. This is a one-line change to the
condition, but it is the difference between a warning that means something and
one that fires on the most common action in the app.

### 5. One confirmation

The toast goes. The button under the user's finger has just become a live status
control for the download they created — the same message, in the place they are
already looking, for as long as they want it.

## Project Structure

```
src/components/SourceTitleModal.vue   # all four changes
e2e/stateful/title-send.spec.ts       # NEW — the flow had no coverage
```

## Phases

**Phase 1** — no default pick; keep the preference meaningful via the tier.
**Phase 2** — the visible-pick invariant.
**Phase 3** — scroll to the next step.
**Phase 4** — season-aware repeat warning; drop the toast.
**Phase 5** — cover the flow end to end, gates, PR.
