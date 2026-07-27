# Feature Specification: Admin settings — edit & test the NAS connection, change password, themes, and glass UI

**Feature Branch**: `feat/1002-admin-settings`

**Created**: 2026-07-27

**Status**: in-review
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Operator request: "It still shows the NAS host in Settings and doesn't let me see and change
the settings I made during the wizard. Add a test-connection when the host/auth changes. Adjust the
manifest/compose env if it's still there. Move relevant settings into their own dive-in section. Enable
the iOS translucency (Liquid Glass) look. Add dark and light themes with dark as the default and no
follow-the-system option. Match the theme color and app icon to the Ring project (admin only for the
connection/account changes)."

## Overview

After first-run setup, the stored operator configuration (public URL, NAS address/port/TLS, NAS
credentials, admin account) was write-once and invisible — Settings showed a stale, read-only host
sourced from the `SYNO_URL` env even in stateful mode. This feature makes those settings **viewable and
editable by admins**, with a **test-connection** that verifies before saving, plus appearance controls
(dark/light theme, iOS glass) and a brand refresh to match Ring.

## User Scenarios

### US1 — Admin edits and tests the NAS connection (P1)
An admin opens Settings → NAS connection (a dive-in editor), sees the current public URL, NAS
address/port/TLS-verify, and account (never the password). They change any field, tap **Test
connection** to confirm the NAS accepts it (2FA prompt when applicable), then **Save** — which
re-verifies and rolls back to the previous working config if the NAS rejects it. Settings' host row then
reflects the stored value, not `SYNO_URL`.

### US2 — Admin changes their own SynoDL password (P2)
An admin opens Settings → Account → Change password, enters a new password (≥8 chars) twice, and saves.

### US3 — Anyone sets the app theme (P2)
A user toggles Dark mode in Settings → Appearance. The choice is a manual dark/light selection; **dark is
the default** and there is **no "follow system" option**. The choice persists and applies before the next
cold launch (no light flash).

### US4 — Glass UI + Ring brand (P3)
Toolbars and the tab bar adopt the translucent "Liquid Glass" look on iOS (Ionic `:translucent` +
fullscreen content), and the accent color + app-icon set match Ring's emerald palette.

## Functional Requirements

- **FR-001** The server MUST expose `GET /v1/nas/config` (admin-only) returning the non-secret operator
  config: `publicUrl`, `nasAddress`, `nasPort`, `nasTlsVerify`, `nasAccount`, `nasUses2FA`. The NAS
  password MUST NOT be returned in any form.
- **FR-002** `POST /v1/nas/test` (admin-only) MUST verify a candidate connection by establishing and
  immediately dropping a real DSM session (persisting nothing) and map DSM failures to the usual typed
  errors. A blank password/field MUST fall back to the stored value so an unchanged-credential re-test
  works.
- **FR-003** `PUT /v1/nas/config` (admin-only) MUST update the stored connection. A change to the
  connection/credentials MUST re-verify by establishing a session and **roll back to the previous config
  on failure**. A non-connection edit (public URL only) MUST save without touching the NAS. A blank
  password MUST keep the stored secret.
- **FR-004** In stateful mode, the Settings host MUST come from the stored operator config
  (`nasAddress`), not from `SYNO_URL`. `SYNO_URL` in stateful mode is only a first-run wizard prefill.
- **FR-005** All three NAS-config endpoints MUST be admin-only (403 for non-admins, 401 unauthenticated).
- **FR-006** An admin MUST be able to change their own SynoDL password from Settings (reusing the
  admin user-update path), requiring ≥8 characters and a confirmation match.
- **FR-007** The client MUST offer a manual dark/light theme with dark as the default and **no
  follow-system option**, persisted so the pre-paint applies it before Vue mounts.
- **FR-008** Relevant settings MUST be grouped into sections, with the NAS connection editor as a
  dive-in (modal) rather than inline rows.
- **FR-009** Deployment manifests/compose MUST treat `SYNO_URL` as OPTIONAL in stateful mode (documented,
  defaulted empty), since the NAS is configured via the wizard.
- **FR-010** The UI MUST adopt the iOS translucent (glass) treatment on toolbars/tab bar, and the accent
  color + PWA icon set MUST match Ring's emerald `#10b981`.

## Credential-Safety Impact (Principle III)

- The NAS password is **write-only across the wire**: `GET /v1/nas/config` never returns it; edits accept
  a blank password meaning "keep the stored secret". The password stays in the encrypted `nas_password_enc`
  column and is only ever decrypted server-side to log in to the NAS.
- `POST /v1/nas/test` persists nothing (login + logout, discard sid). Test/edit both hold the sid only for
  the connection's life; no credentials/OTP/sid are logged.
- All edit/test/read endpoints are admin-guarded server-side (not merely hidden in the UI).
- No new DSM API is added to the allowlist; test/verify reuse the existing `Login`/`Logout`. The stateless
  proxy boundary is unchanged; stored state remains custodial (constitution v2.0.0).
- A rejected connection edit rolls back to the last working config, so a bad edit cannot strand the
  instance.

## Out of Scope

- Rotating `SECRETS_KEY` or re-keying encrypted columns.
- Changing OTHER users' passwords from this screen (existing admin user management already covers that).
- A framework "Liquid Glass" theme (Ionic ships none; this uses native `:translucent` + CSS).
