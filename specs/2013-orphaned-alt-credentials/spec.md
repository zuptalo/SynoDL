# Feature Specification: Removing an address removes its credentials

**Feature Branch**: `fix/2013-orphaned-alt-credentials`

**Created**: 2026-09-05

**Status**: in-review

**Input**: Found while verifying the two-address fallback in action: a source with no alternate address still reported `hasAltSession: true`.

## Context

Spec 0009 gave a source two addresses, each able to carry its own sign-in. It
required that removing an address removes the material stored for it — and that
requirement was not implemented.

The result is visible in practice. An operator who set an alternate address,
pasted its sign-in, then promoted that address to the main one and cleared the
alternate, is left with credentials for an address the source no longer has.
Nothing can ever send them, and the admin screen reports them as present, which
says something untrue about where the source can sign in.

## Requirements *(mandatory)*

- **FR-001**: When a source has no alternate address, no material for one MUST be
  stored.
- **FR-002**: Removing the alternate address MUST remove its material in the same
  save.
- **FR-003**: The source's own single set MUST be untouched by this.
- **FR-004**: Adding the address again MUST require the material to be given
  again, rather than resurrecting what was removed.

## Success Criteria *(mandatory)*

- **SC-001**: A source with no alternate address never reports alternate material.
- **SC-002**: No credential is retained that nothing can send.

## Credential-Safety Impact

- Strictly narrowing: this deletes stored material that is unreachable, and
  removes a presence report that was misleading. Nothing new is stored, returned
  or logged.
