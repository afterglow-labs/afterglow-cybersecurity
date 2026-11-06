# APTEX and Odysseus: client startup and server architecture

Inspection and measurements on 2026-11-06 support keeping APTEX's Go execution
engine and improving the client delivery path. A Python server rewrite would
not, on its own, reduce the JavaScript a connecting client must download.

## What differs today

| Area | APTEX | Odysseus inspected at `7a7d6faea699` |
| --- | --- | --- |
| Server | Go HTTP service with PostgreSQL-backed task execution and graph stores | FastAPI with SQLAlchemy session storage; SQLite by default, configurable database URL |
| Interface | Embedded Next.js static export, React hydration, route chunks | HTML and browser ES modules with static assets |
| Text delivery before this change | Uncompressed static files without explicit cache policy | Gzip middleware, with streaming events excluded |
| Asset freshness | Now: immutable hashed Next.js assets; HTML and unversioned files revalidate with ETags | Unversioned JS, CSS and HTML revalidate to avoid stale application code |
| Optional startup work | Must keep model execution separate from interface readiness | Optional warmups are disabled by default and background tasks are scheduled separately |
| Client deployment | Independent APTEX clients connect to its desktop server | Independent Odysseus clients connect to its server |

APTEX's existing planner, workers, task and asset graphs, traffic capture,
interception, evidence, scheduler, provider integrations and permissions are
coupled to its current execution and persistence APIs. A rebuild would require
porting and verifying that behavior, including existing user data. It could make
future Odysseus feature ports easier, but introduces a broad migration and a
different runtime to maintain. Neither system's implementation language is a
useful explanation for the measured interface delay.

The useful patterns to adopt now are compressed static delivery, deliberate
cache policy, a direct workspace entry point, and optional work outside the
initial render. APTEX and Odysseus retain separate services, accounts and data.
This change adds an independent delivery implementation; it imports no Odysseus
source and adds no dependency on an Odysseus installation.

## Measurement

The baseline was APTEX `187eb0a16d72`, already running locally. The candidate
served the same built application through the new static handler on a temporary
loopback port, proxying its API to the running server. This did not start another
agent engine. Tests used a fresh headless Chrome context, a signed-in Afterglow
workspace, blocked service workers, and a 1440 × 1024 viewport. No model inference
was requested.

The slower connection was emulated with 6 Mbps downstream, 3 Mbps upstream and
50 ms latency. Readiness means the workspace's Preferences button became
visible. Wire bytes came from completed Chrome DevTools network requests,
including the initial interface requests after rendering. Each cell is one
controlled sample, not a production percentile or a Mac Wi-Fi benchmark.

| Entry and connection | Before | After |
| --- | --- | --- |
| `/`, local, workspace visible | 282.0 ms | 273.8 ms |
| `/`, 6 Mbps / 50 ms, workspace visible | 4,879.9 ms | 1,717.2 ms |
| `/function/tasks`, 6 Mbps / 50 ms, workspace visible | 4,764.0 ms | 1,666.1 ms |
| `/`, cold transferred bytes | 3,817,016 | 996,890 |
| `/function/tasks`, cold transferred bytes | 3,088,697 | 996,890 |

The root-entry sample transferred about 74% fewer bytes and reached the
workspace about 65% sooner on the emulated slower connection. Local workspace
timing did not materially improve. The server's local health response was
approximately a millisecond; an already idle model provider was not responsible
for this measured download delay.

A subsequent authenticated reload in the candidate browser context reached the
workspace in 173.8 ms under the same emulated slower connection, with no static
bundle bytes transferred. Preferences opened and closed successfully and no
JavaScript runtime errors were observed.

Odysseus's unauthenticated landing page redirected to its login page in this
inspection. Its paint timings are **not** a like-for-like comparison with APTEX's
authenticated workspace and are excluded from the before/after table.

## Changes and remaining work

- Static builds write Brotli and gzip variants of text assets. The server
  negotiates an acceptable encoding and keeps the original MIME type.
- Only public static files use this handler. Authenticated API payloads and SSE
  streams do not gain compression or public caching from this change.
- Content-hashed `/_next/static/` assets use immutable caching; HTML and
  unversioned files revalidate. ETags differ between encoded representations.
- `/` redirects to `/function/tasks` on the server, avoiding an extra JavaScript
  bootstrap. The Mac client starts at that workspace route directly.
- The Mac app opens its native window before connecting, uses system WebKit and
  contains no server, container stack or model runtime.

All six locale catalogs still load with the shared interface code. Loading only
the selected locale is a potential follow-up; it needs careful handling of
language switching and hydration. Large graph features already have separate
route chunks and did not explain the initial task-page transfer in this test.
Further work should measure real desktop-to-Mac network conditions and
route-specific rendering before changing the engine.

## Verification

The full Go test suite passed against a disposable PostgreSQL database, followed
by `go build ./...` and `go vet ./...`. Frontend installation, all 89 frontend
tests and the static export passed. HTTP tests cover encoding preferences,
decoded content, MIME types, missing assets, redirects, ETags and cache policy.
The native Mac package compiled as a universal binary and passed URL-boundary
tests and a real WebKit fixture launch on Apple Silicon and Intel CI runners.
Desktop-to-Mac connectivity still requires the user's reachable server address
or an SSH tunnel; the default server binding remains desktop loopback.
