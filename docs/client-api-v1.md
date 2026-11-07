# APTEX client API, version 1

The application API is independent of Odysseus. Its accounts, signing keys,
database, model configuration and lifecycle belong to APTEX. A native client
connects to an existing server; it does not start a server or download models.

## Connection

1. Fetch `GET /api/v1/meta` without credentials. Require `service: "aptex"` and
   `api_version: "1"` before asking for or sending a password. The response gives
   relative API, authentication, readiness and capability paths.
2. Check `GET /api/ready`. HTTP 200 means required database access is available;
   HTTP 503 means the API is not ready. Models, browser workers and MCP services
   are optional. `/api/health` remains the lightweight process/version probe.
3. Sign in, then load `GET /api/v1/capabilities`. Load feature data as its screen
   opens. Connection must not trigger inference or tool discovery.

Use HTTPS for remote connections. Existing deployment Tailscale Serve routes
remain independent of Odysseus. No credentials may appear in v1 request URLs,
including SSE URLs. Requests with a `token` query parameter are rejected.

## Authentication

`POST /api/v1/auth/login` accepts JSON `{ "username": "Afterglow", "password":
"…" }`. A successful response contains `token`, `username`, and RFC 3339
`expires_at`. Each login creates a distinct seven-day session. Native clients
send `Authorization: Bearer <token>` and store the opaque credential in their
own server-scoped Keychain entry. Never persist passwords or log tokens.

`GET /api/v1/auth/session` validates the session and returns `user`, `expires_at`
and `api_version`. HTTP 401 requires sign-in. HTTP 503 means session storage is
unavailable; retain the saved credential so a transient outage does not sign
the user out.

`POST /api/v1/auth/logout` revokes that session and clears its browser cookie.
It is safe to repeat. Other device sessions remain valid. A password change
through the existing authenticated password route atomically revokes all
interactive sessions, including unregistered legacy tokens. Short-lived scoped
gateway credentials retain their separate existing expiry and route restrictions;
they cannot use a revoked parent session to mint another credential.

The database stores SHA-256 token hashes and revocation records, never raw bearer
credentials. Authentication fails closed when required storage is unavailable.
Live observers recheck revocation; disconnecting an observer does not stop a job.

### Browser transition

Login sets an HttpOnly, SameSite=Lax cookie, with Secure on HTTPS. New web sessions
persist display metadata rather than bearer credentials in localStorage.
`POST /api/v1/auth/restore` validates the current cookie, or a legacy bearer header,
and establishes the HttpOnly cookie. Existing valid administrator JWTs keep their
remaining lifetime until logout or password change; ARTEX identities remain
invalid. A restored legacy session cannot override its revocation record.

Cookie mutations require a matching origin. For development across ports, set
`APTEX_ALLOWED_ORIGINS=http://localhost:3000` (using the actual frontend origin)
on the backend. Only explicitly listed origins receive credentialed CORS.
Alternatively set `NEXT_PUBLIC_SSE_BASE=""` and use a same-origin development
proxy that streams SSE. Production clients use their server's origin directly.

## Existing cybersecurity resources

Existing `/api/` resources are also available under `/api/v1/`, preserving their
DTOs, IDs, pagination, error envelopes and authorization rules. Tasks, findings,
evidence, graphs, workflows, traffic, interception, agents, conversations and
model profiles continue to use the existing stores and execution engine.
Identifiers must retain their full precision: task/finding IDs are strings;
conversation and numeric profile IDs require signed 64-bit integers. Revisions
and SSE cursors are decimal strings.

`POST /api/v1/tasks` creates **and starts** a task. Present that action as
"Create and run". Use the full console for advanced scope and interception
configuration. Available models and access flags come from server profiles;
clients must not assume a single model or offer unsupported access programs.

Chat inputs use the existing durable endpoint
`POST /api/v1/conversations/{id}/controls/inputs`. Retain the same
`client_request_id` and identical request body when retrying an ambiguous
network failure. Fetch `/controls` before pause/resume and send its
`expected_revision`. On HTTP 409, refresh the state and show the conflict;
never silently stop a newer run. Backgrounding a client does not cancel work.

## Resumable conversation events

`GET /api/v1/conversations/{id}/events` is authenticated SSE. Send
`Last-Event-ID: <decimal cursor>` when reconnecting, or the non-secret `cursor`
query parameter. The server reads persisted active conversation activities after
that cursor. Closing the connection cancels only the observer.

| Event | Data / behavior |
| --- | --- |
| `activity` | Existing ActivityDTO; SSE `id` is its decimal sequence. Deduplicate by sequence. |
| `state` | `cursor`, `running`, `paused`, `history_revision`, `control_revision`, `context_changing`, and optional `active_run_id`. Sent when state changes. |
| `heartbeat` | Current cursor, approximately every 15 seconds. |
| `session_expired` | Stop reconnecting and request sign-in. |
| `unavailable` | Storage/state unavailable; retry with bounded backoff. |

Load the current paginated `/messages` and `/controls` snapshots before observing.
Refresh active history when `history_revision` changes, including after an edit,
retry or rollback. Preserve the draft and selected model while doing so. A cursor
replays active persisted activities; it is not an immutable audit stream of
deactivated attempts. Use pagination for older history. Retry with backoff after
network interruption and after returning from the background. The existing
PostgreSQL admission/recovery rules remain authoritative across server restarts.

## Runtime boundary

The launcher starts PostgreSQL and the API without requiring model weights. In
automatic mode it starts local Ollama when a verified local store is configured
and the selected provider uses that runtime. `APTEX_LOCAL_MODEL_ENABLED=false`
disables launcher-managed Ollama; `true` requires a valid model directory.
Existing local installs keep their read-only model mount and network policy.
An unavailable selected model is reported without taking the API down.

These are application boundaries inside the existing Go server. Execution has
not been extracted into a second process, and no Python server rewrite is needed
for this client contract.
