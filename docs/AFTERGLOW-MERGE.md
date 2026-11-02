# Afterglow merge decisions and verification

This record covers the original checkout and the expanded integration. ScopeWeaver is the base because it has the broadest maintained Go and Next.js application, task graph, six-language UI, MCP management, interception controls, and test coverage. The initial import is commit `634e705`; the later feature work listed here is awaiting a full dependency-backed build.

| Input | Inspected commit | Integrated capability |
| --- | --- | --- |
| ScopeWeaver | `a89795a` | Base application, task and asset graphs, planner and workers, interception, traffic store, notifications, model profiles, agent triggers, MCP, and six UI languages. |
| Artex-on-ChatGPT | `afcd6ca` | ChatGPT OAuth/PKCE connection, encrypted private account store, refresh and quota checks, account model catalog, Responses adapter, plan profiles and UI, browser sessions, image messages, and provider-data compaction fixes. |
| artex-browser-chat | `13fabbb` | Authenticated traffic extension, workflow canvas and scheduling, operator questions, encrypted test accounts, task outcomes, lessons and skill gaps, optional Telegram stdio MCP helper, and imported playbooks. |
| artex-deep-hunter | `abbe8e4` | Defensive detections and triage, bounded autonomous follow-up waves, cross-task confirmed finding hints, business map, hypothesis campaigns, opt-in hunt-class coverage board, attack-chain composition and verification tasks, scoped native request replay, stall watchdog, and focused hunting skills. |
| KSIR-ARTEX | `240ae7c` | Operator-managed proxy graph, local artifact manifest, C2 handoff records, scoped cloud assessment and lateral movement planning tools, and additional playbooks. |

## Security decisions

- ChatGPT account tokens stay in a dedicated local store and Docker volume; PostgreSQL holds account references. OAuth uses state, nonce, PKCE, issuer/JWKS validation, and a fixed loopback callback. Plan inference fails closed when the account is unavailable and does not silently use an API key. The local-only model policy blocks plan inference and catalog requests. The auth key has its own durable volume and startup fails if it is invalid or cannot be persisted.
- Browser profiles live outside browsable task workspaces. The browser traffic extension sends its token only to HTTPS or loopback HTTP addresses, and the ingest endpoint is authenticated and size-limited.
- Workflow schedules and graph size are bounded and validate their agent and timezone. The source fork's hardcoded public connector was removed.
- Follow-up waves are opt-in, delayed, time-boxed, and capped at three per root task. Cross-task hints use confirmed findings from the same scoped surface. Proposed attack chains require confirmed finding references; a separate verification task records a verified or rejected result. The hunt-class board is opt-in because many task types do not fit a full web vulnerability checklist. Open classes and untested campaign branches hold task completion once recorded.
- Proxy nodes are operator-defined; a failed validation does not fall back to direct target traffic. C2 handoff requires a manual task scope entry and an operator acknowledgement; it does not copy, deliver, or execute a binary. `exfil_data` indexes actual files in the task workspace with hashes and no outbound transfer. Cloud and lateral tools return scoped plans or inspect local evidence and never report unperformed actions as successful.
- The optional Telegram plugin runs as a manually connected stdio MCP server. API credentials come from the environment, session files stay private, and `TELEGRAM_ALLOWED_CHATS` gates sending. The source's embedded API credential and unauthenticated HTTP wizard are absent.
- Native race, identity, and inflight request replay requires an operator-entered target scope, a passing asset interception gate, and an active recording proxy. The guard sends each replay call through interception, asking the operator by default and denying on timeout. Every distinct destination is checked before a batch starts, so scope checks do not stagger a race burst. Batches have request, body, and time limits; redirects are not followed, and missing capture fails closed. Guest identity replay strips credential headers, and candidate IDs are escaped inside URL, JSON, and form values.
- The capture proxy now verifies upstream target TLS certificates by default. An isolated lab can explicitly set `ARTEX_TRAFFIC_INSECURE_UPSTREAM=true` for a self-signed target. The imported origin, race, SSRF, and chaining playbooks were rewritten to use scoped evidence and bounded tools instead of direct raw-socket bursts, origin-range scans, or metadata-token retrieval.
- Source fork AMSI-bypass, reflective credential-loader, PHP web-shell/bulk-exploit, and WAF mutation files are preserved under `extras/source-assets/` for provenance. They are outside the runtime skill directory and excluded from the Docker build context. The active PHP and upload skills use inert-marker assessment workflows, the WAF skill respects rate limits and challenge responses, and CVE claims require current vendor advisory verification.
- KSIR's mock `deploy_payload`, `establish_persistence`, `lateral_move`, and `exfiltrate` success messages were not retained. Those source tools claimed outcomes without performing them. The handoff, assessment, and local artifact tools report only actual recorded work.
- Imported playbooks carry source attribution and local-scope notes. The dated CVE snapshot was replaced by a live-source playbook. The original release/update workflow does not point back to ScopeWeaver; release publication requires a reviewed Afterglow workflow.

## Verification on this checkout

| Check | Result |
| --- | --- |
| `node extension/background.test.mjs`; extension script syntax | Passed (five focused tests). |
| Browser runtime script test | Could not bind `127.0.0.1` in this sandbox (`listen EPERM`); rerun where loopback sockets are allowed. |
| Detection indicator, ATT&CK, harness-sync, and host-triage checks | Passed in the initial import. Full Docker detection suite unavailable here. |
| Pure Go packages: `artifactindex`, `cloudassess`, `c2bridge`, `proxygraph` | Passed with `GOPROXY=off`. |
| Native replay helper security tests | Passed in an isolated copy with local tool stubs; this checks URL/body escaping and fail-closed branches, not full worker integration. |
| Frontend localization suite | 23 of 24 checks passed. The remaining formatter test cannot load absent `date-fns`; catalog parity and Han guards passed. |
| Telegram helper Python syntax | Passed with `python3 -m py_compile`. |
| Full Go backend tests/build | Not run successfully: required modules are missing from the local cache and network/module-cache writes are unavailable in this sandbox. Database-backed tests additionally need a disposable PostgreSQL instance. The native replay tests in the actual package still need to run there. |
| Full frontend install/build | Not run successfully: the npm cache lacks required packages, including `date-fns`. Browser end-to-end and Docker checks remain outstanding. |

The passing checks validate only the packages and static content named above. Run `go test ./... -count=1` with `ARTEX_TEST_DSN`, `npm ci && npm test && npm run build:static` in `web/`, and the Docker detection suite before treating this working tree as a release.
