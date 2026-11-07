# Native client comparison

Reference: the independent `afterglow-labs/odysseus-ios` repository at
[`f81c2cd09b90c49c308343c7569f795de2bf40c5`](https://github.com/afterglow-labs/odysseus-ios/tree/f81c2cd09b90c49c308343c7569f795de2bf40c5),
version 0.5.4 (69). The comparison uses its current `RootView`, `ChatView`,
`MessageView`, `ModelControlsView`, networking, credential vault and UI tests.

| Client behavior | APTEX implementation |
| --- | --- |
| iPhone navigation and iPad conversation sidebar | Compact chat stack; regular-width split navigation; native Tasks, Chat, Findings, Console and Settings tabs |
| Native readable messages | Fetches complete message details, renders Markdown and fenced code, supports selection and raw-message copy |
| History and reconnect | Persisted server history, earlier-page loading, durable SSE cursor, background/reconnect refresh and revision checks |
| Agent and model choice | Server agent list, visible model chip and profile picker; advanced provider/access-program controls in Console |
| Photos and documents | System photo picker and document picker with downloaded copies, size limits, cancellation and protected temporary images |
| Composer continuity | Drafts and attachments survive tab, theme and language changes; unconfirmed submissions retain their request ID and content in server-scoped Keychain storage |
| Session handling | APTEX identity check before credentials, dedicated server-scoped Keychain service, saved-session restore and per-device revocation |
| Appearance and language | Three themes, dark/light/system mode, English primary and six supported native UI languages |
| Existing server features | Authenticated ephemeral WebKit Console for graphs, workflows, traffic, interception, profile configuration and advanced assessment controls |
| Exports and external links | HTTP and browser Blob exports use the system share sheet; same-origin navigation stays authenticated; external HTTPS opens in the system browser |
| Installation | Separate `com.afterglowlabs.aptex` app, iOS/iPadOS 17+, client-only arm64 IPA for the owner's Feather signing workflow |

Native networking is validated against both an owned deterministic fixture and
an isolated real Go backend over HTTPS. Simulator unit and UI results, including
iPhone/iPad coverage, are recorded in [verification.json](verification.json).
Simulator evidence does not establish installation or runtime behavior on a
physical device; release delivery records distinguish transfer from installation.

Odysseus remains an independent application. APTEX uses its own server, accounts,
assessment storage and model configuration. New ChatGPT account authorization
uses the desktop's existing local callback flow; connected profiles are then
available to the client.
