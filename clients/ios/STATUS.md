# iOS client work paused

The owner paused client implementation on 2026-11-06 so APTEX's server
architecture and client API can be settled first. This branch preserves the
initial inspected/adapted helpers only. It is not a runnable iOS application,
and no iOS build or release has been published.

Inspected source: `afterglow-labs/odysseus-ios` at
`f81c2cd09b90c49c308343c7569f795de2bf40c5` (AGPL-3.0). The older `ios/` copy
inside the Odysseus server checkout is not the current native client.

The existing `afterglow-mac` SSH configuration reaches the verified Mac build
machine. Existing iPhone and iPad simulators are available, and the established
iPhone delivery method is a Feather-compatible IPA. Reverify paths, device
availability and signing configuration before resuming; do not change or reuse
Odysseus's app identity or Keychain namespace.

Resume client implementation after the server's session/authentication,
background-job, capabilities and API-version boundaries are decided.
