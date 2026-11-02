# Afterglow merge decisions and verification

This file records the source comparison for the initial Afterglow checkout. All five input repositories were clean at the inspected commits. The destination had no commits or files before this import.

| Input | Inspected commit | Decision |
| --- | --- | --- |
| ScopeWeaver | `a89795a` | Base. It has the broadest tested backend and maintained frontend, six UI languages, task source relations, work steering, agent triggers, MCP management, and source/archive controls. |
| artex-browser-chat | `13fabbb` | Imported the traffic capture extension and authenticated ingest path. Its workflow canvas used a hardcoded public Cloudflare tunnel and a separate workflow table; ScopeWeaver's local agent triggers and MCP management are the supported equivalent. The Telegram plugin contained a hardcoded API credential and was not imported. |
| artex-deep-hunter | `abbe8e4` | Imported defense rules, ATT&CK layer, indicator exports, host triage, and three focused skills. Updated the self-update User-Agent and guard marker signatures to match the ScopeWeaver base, then checked source-to-indicator consistency. Automatic follow-up waves were not ported because they alter task lifecycle and database scheduling and need integration tests against a live PostgreSQL instance. |
| KSIR-ARTEX | `240ae7c` | Imported five reusable playbooks. The C2, exfiltration, residential/Tor proxy rotation, and stale static CVE snapshot were not imported into this local-only research console. |
| Artex-on-ChatGPT | `afcd6ca` | Reviewed its ChatGPT subscription provider, browser sessions, and runtime fixes. The subscription provider spans separate auth and LLM modules plus database, profile, API, and UI changes; it was not mixed into ScopeWeaver's different provider-routing and local-only controls without an integration test environment. The base already contains extensive chat recovery, context, and work-steering behavior. |

## Integrated code

- `server/traffic_ingest.go` and `traffic/external.go` accept capped, authenticated browser exchanges into the existing SQLite-backed traffic store. The route remains behind the app's JWT middleware. HTTP methods, URL schemes, status codes, and body sizes are checked; header names and values are sanitized. `Authorization` is allowed in CORS preflight for the extension.
- `extension/` flushes all pending batches on stop, periodically flushes active capture, keeps its bounded unsent queue in session storage across service worker restarts, and sends its JWT only to HTTPS or loopback HTTP URLs. The frontend CI job runs its Node checks.
- `detections/`, `docs/defense-en.md`, and `docs/defense-ko.md` bring over the defensive content. Two exact indicators were updated to the base's emitted values: `scopeweaver-selfupdate` and `[ScopeWeaver platform control, not target defense]`. The rule filenames retain their original ARTEX names for compatibility with the detection suite.
- `skills/` now includes `api-security`, `authz-hunter`, `codeaudit`, `llm-pentest`, `oauth-attack`, `patch-diff`, `rce-hunter`, and `xss-hunter` in addition to the three base skills. Each added playbook has parseable frontmatter and an Afterglow local-only scope note.
- The UI's login, setup, sidebar, favicon, and primary metadata use Afterglow branding across all six shipped languages. Internal API and executable names remain compatible with the base.
- `selfupdate/github.go` points only to `afterglow-labs/afterglow-cybersecurity`; a future release cannot silently update this merged app from the ScopeWeaver upstream repository. Inherited workflows that would publish the old landing site or release image were removed, along with the obsolete v0.2.1 release-repair guide. The `landing/` site remains as an upstream reference and is not published by this checkout. Release publication for Afterglow needs its own reviewed workflow.

## Verification on this checkout

| Check | Result |
| --- | --- |
| `node extension/background.test.mjs` | Pass: five focused tests. |
| `node --check extension/background.js` and `popup.js` | Pass. |
| `ARTEX_REPO_ROOT="$PWD" python3 detections/tests/indicators/check.py` | Pass: emitted strings, rules, CSV, and CI/hook coverage. |
| `ARTEX_DETECTIONS_DIR="$PWD/detections" python3 detections/tests/attack/check.py` | Pass: layer and Sigma technique/tactic mapping. |
| `ARTEX_REPO_ROOT="$PWD" python3 detections/tests/check-harness-sync.py` | Pass: eight suites match CI and the aggregate runner. |
| `python3 detections/triage/artex_host_triage.py --self-test` | Pass: synthetic host evidence and clean-host cases. |
| JSON parsing for MISP event and ATT&CK layer; Go `gofmt` on new Go files | Pass. |
| Workflow YAML parsing, `docker compose config --quiet`, and inherited landing validator | Pass. |
| Go `go test ./traffic ./server` | Could not run: required Go modules were absent from the local cache and network access was blocked. |
| Web `npm ci --offline` / build | Could not run: `zwitch-2.0.4.tgz` was absent from the npm cache. |
| Full Docker detection suite | Could not run: the sandbox cannot access the Docker socket. MISP's direct check also needs `pymisp`; the guide's inline Sigma check needs `sigma`. |

The passing checks establish the imported extension behavior and detection data consistency. They do not establish a successful full Go or Next.js build. Run the repository CI and the full detection suite in an environment with dependencies and Docker before calling this a release.
