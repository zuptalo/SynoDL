# Feature Specification: Per-user destination preferences on the server, with self-cleaning

**Feature Branch**: `feat/1011-destination-prefs-server`

**Created**: 2026-07-28

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Operator report: "The default and favorite folders I choose don't survive full app closure.
They should persist in the database for the user, so changing them in one session updates them
everywhere. And if a favorite or default folder no longer exists on the NAS, or access was revoked by
the admin, it should just go away — and the default should reset to root."

## Functional Requirements

- **FR-001** A user's default destination + favorites (≤4) are stored PER-USER on the server
  (`destination_prefs`, migration 0004), so they survive app closure and sync across the user's sessions.
- **FR-002** `GET`/`PUT /v1/destinations/prefs` (self, authenticated) read/write the prefs.
- **FR-003** On read, the server drops any favorite/default the user may no longer use (grant revoked)
  or that no longer exists on the NAS; the default resets to root (empty) when it's gone. The cleaned set
  is persisted so the removal sticks.
- **FR-004** Existence is checked by listing each folder's parent and confirming membership; a NAS
  outage MUST NOT wipe prefs (only in-memory access checks apply then).
- **FR-005** On write the server normalizes, de-duplicates, caps favorites at 4, and drops any the user
  isn't allowed to use.
- **FR-006** In legacy/stateless mode (no per-user server storage) the client degrades to in-memory for
  the session.

## Notes

Existing client-side (IndexedDB) favorites are not migrated; a user re-picks once and it then persists
server-side.

## Testing

Server: persist + round-trip; existence cleanup (gone default resets to root, missing favorite dropped);
access-revocation drop on read; auth required. Client uses the API with an in-memory fallback.
