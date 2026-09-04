# Feature Specification: PWA install guard — sign in only from the installed app

**Feature Branch**: `feat/1008-install-guard`

**Created**: 2026-07-27

**Status**: shipped
<!-- SynoDL spec lifecycle: planned → in-progress → in-review → shipped. -->

**Input**: Operator request: "Give SynoDL the same install instruction we have for Ring, so users can
only log in if they open the app as a PWA."

## User Scenario

Opened in a plain browser tab at the public origin, SynoDL shows a full-screen **install guide** instead
of the login screen; the user can only proceed once they've added it to the Home Screen (installed it as
a PWA). Running as the installed app (standalone display mode), the guide never appears.

## Functional Requirements

- **FR-001** When the app is NOT in standalone display mode and NOT on localhost, a full-screen install
  guide MUST overlay the app, blocking sign-in.
- **FR-002** The guide MUST offer platform-appropriate steps (iOS / Android / desktop), a native install
  button when the browser exposes `beforeinstallprompt`, and accurate callouts for Android WebviewViews
  and Firefox-on-Android (which can't install a real PWA).
- **FR-003** localhost MUST be exempt so local development and the e2e suite are never blocked.
- **FR-004** When the app becomes standalone (or fires `appinstalled`), the guide MUST drop.
- **FR-005** No public origin/host is hard-coded in the guide (guidance is generic).

## Testing

The pure UA detection (`detectPlatform`, `isAndroidWebView`, `isFirefoxAndroid`) is unit-tested on the
coverage allowlist. The standalone/localhost gating is DOM-driven; the e2e suite (on localhost) is exempt
by design, confirming the bypass.
