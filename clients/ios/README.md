# APTEX for iPhone and iPad

A native SwiftUI client for **APTEX by Afterglow Labs**. Your desktop runs the
server, models and assessments. The iOS app connects to it over your existing
private network or Tailscale DNS endpoint. It installs independently of Odysseus.

## Install and connect

1. Download `APTEX-iOS-Feather.ipa` from the
   [iOS client release](https://github.com/afterglow-labs/afterglow-cybersecurity/releases/tag/aptex-ios-client-v1.0.0).
2. Import the IPA into Feather and sign/install it using your existing signing
   setup. The packaged app is ad-hoc signed for this workflow; it is not an
   App Store or TestFlight installation.
3. Enter the desktop's APTEX HTTPS address, including its port. Use the APTEX
   account, normally **Afterglow**, and its existing password. Tailscale must be
   connected when using a tailnet DNS address.

The app requires iOS/iPadOS 17 or newer. Its bundle ID is
`com.afterglowlabs.aptex`. The Mac client and Odysseus keep their own identities.
Remote addresses require HTTPS. Private network/VPN and loopback HTTP addresses
are accepted; certificate validation stays enabled. The default server Compose
port is published only on desktop loopback, so use the configured private proxy
endpoint or an appropriate private tunnel to reach it from another device.

## Workflows

| Screen | Available functionality |
| --- | --- |
| Tasks | Search, create and run, inspect goals and scope constraints, pause/resume, open full task details |
| Chat | Visible agent/profile selection, full message history with Markdown and code, copy, search, resumable live events, queued messages, revision-checked pause/resume, files and photos |
| Findings | Paginated findings, severity/status, evidence and reports |
| Console | Full existing web console, including graphs, workflows, traffic, interception, model configuration and advanced controls |
| Settings | Server/account information, saved-session restore, server switching, logout, themes, language and licenses |

Native themes are Obsidian, Phosphor and Afterglow, each with dark, light and
system appearance. English is the default. Native labels and the network
permission support Korean, Simplified Chinese, Traditional Chinese, Japanese
and Spanish as well. Server records and user-authored content retain their
original language. The Console also retains the existing web language and theme
controls.

Chat uses a conversation sidebar and detail pane on iPad, and stacked navigation
on iPhone. The [Odysseus comparison](odysseus-parity.md) records the reference
client and the corresponding APTEX workflows.

Models and access programs come from the server's configured profiles. The app
uses connected providers, including local models and ChatGPT-plan profiles.
New ChatGPT account connections are managed on the desktop: the server preserves
its existing loopback-only account-management and OAuth callback restrictions.
Configure those accounts there, then select or configure their profiles in the
client's Console. The app never downloads a model or starts a local server.

## Session and reconnect behavior

The app verifies the APTEX identity before sending a password. Session credentials
use APTEX's own server-scoped Keychain entries. Passwords are not saved. Native
requests use bearer headers, and the private, ephemeral WebKit console receives
an HttpOnly cookie directly. Credentials never enter URL parameters, JavaScript
or web localStorage. HTTPS redirects cannot forward native credentials to another
origin. There is no native command bridge.

Disconnecting or backgrounding the phone stops observation while work continues
on the server. On return, the app refreshes persisted history and resumes events
from its cursor. Unconfirmed sends keep their original request ID, message and
attachment metadata in the device's Keychain, including across process restarts;
a retry cannot create a second admission. Logout clears those saved inputs and
revokes the current session. Switching servers preserves separate saved logins
and server-owned jobs. A failed task-creation response asks the user to inspect
Tasks before creating another assessment.

Native uploads support files and photos up to 32 MB with cancellation; the Console retains the server's larger
upload limit and advanced workflows. Console exports use the system share sheet;
temporary exports receive complete file protection and are removed after sharing.
External HTTPS links open in the system browser. Native unsent drafts remain in
memory while switching tabs, themes and languages; confirmed pending sends have
the separate Keychain recovery described above.

## Build and test

From the verified repository root on a Mac with Xcode:

```sh
python3 clients/ios/scripts/check-localizations.py
python3 clients/ios/scripts/generate_project.py
clients/ios/scripts/test.sh <booted-simulator-UDID>
clients/ios/scripts/package.sh
```

The scripts resolve files relative to their own directory. The test script owns
a synthetic loopback fixture on port 18768 and refuses to attach to an existing
process. Tests never contact a model provider, use production credentials, or
start an assessment on the desktop. Simulator tests use ad-hoc signing so the
actual Keychain behavior is exercised. iPhone and iPad CI run independently.

Packaging builds an arm64 device app, verifies its bundle identity and code
signature, and records the source revision in its Info.plist. Outputs and SHA-256
checksums go to `clients/ios/build/release/` by default. Set `APTEX_OUTPUT_DIR` to
choose another existing or new output directory. For an exported source tree
without Git metadata, provide the full committed `APTEX_SOURCE_REVISION`.

The [native contract checker](scripts/contract-check.swift) compiles the same
networking sources on a Mac and exercises an isolated real Go backend over HTTPS.
It accepts synthetic fixture configuration through stdin and refuses any server
whose version is not `platform-candidate`. It covers metadata, authentication,
resource DTOs, conversation creation, attachment upload, SSE state and revocation.

## Provenance

Credential storage, path helpers, SSE parsing, Markdown/code presentation,
attachment import and the dependency-free Xcode/test script structure were adapted from
[afterglow-labs/odysseus-ios at f81c2cd09b90](https://github.com/afterglow-labs/odysseus-ios/tree/f81c2cd09b90c49c308343c7569f795de2bf40c5).
APTEX's versioned API, persisted event cursors, retry recovery, cybersecurity
screens and Console transport are implemented for this server. Adapted files
retain source notices; the app bundles the AGPL-3.0 license. The iOS icon renders
the existing APTEX vector mark with an opaque background.
