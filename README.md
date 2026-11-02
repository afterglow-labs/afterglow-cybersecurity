# Afterglow Cybersecurity

Afterglow Cybersecurity combines selected features from related ARTEX-derived projects into one self-hosted security research console. Five forks were reviewed; ScopeWeaver supplies the base, with integrated code and content from three others. The application uses a Go backend, a Next.js frontend, PostgreSQL, a shared asset graph, and per-task exploration graphs. It is intended for locally isolated, authorized research environments.

## What is included

| Source | Integrated capability |
| --- | --- |
| ScopeWeaver `a89795a` | Core application: planner and workers, asset and evidence graphs, task controls, live work steering, MCP connections, scheduled agent triggers, local and hosted model profiles, six UI languages, and the agent adapter. |
| artex-browser-chat `13fabbb` | Browser extension that records an authenticated tab's HTTP exchanges through `POST /api/traffic/ingest` into the existing traffic store. |
| artex-deep-hunter `abbe8e4` | Defensive Sigma and Suricata rules, ATT&CK layer, indicators, host triage utility, and the `authz-hunter`, `xss-hunter`, and `rce-hunter` skills. Detection fingerprints were aligned with this application's source. |
| KSIR-ARTEX `240ae7c` | `api-security`, `codeaudit`, `llm-pentest`, `oauth-attack`, and `patch-diff` skills. |

The ScopeWeaver base already provides work steering, task source inheritance, agent triggers, and MCP management. The browser-chat workflow canvas depended on a hardcoded external connector endpoint, so the existing local trigger and MCP flows are the supported path here. The ChatGPT subscription adapter, Deep Hunter's automatic follow-up waves, and KSIR's C2, exfiltration, and proxy rotation modules are not part of this integration. See [merge decisions and verification](docs/AFTERGLOW-MERGE.md) for the code-level review.

## Start locally

Docker and Docker Compose are required for the standard installation. Copy `.env.example` to `.env`, set `POSTGRES_PASSWORD`, then build this checkout:

```sh
cp .env.example .env
docker compose up -d --build
```

Open `http://localhost:8787` and create the administrator password at `/setup`. Configure a model in **System → LLM** before starting an agent task. The Docker service and executable still use the internal `scopeweaver` name, and environment variables retain `SCOPEWEAVER_*` and `ARTEX_*` names for compatibility with the base. `docker compose logs -f scopeweaver` shows startup logs.

For other installation modes and detailed operation, consult the [ScopeWeaver baseline guide](README.scopeweaver.md). Its release links and one-click updater refer to the original project; the updater in this checkout is pinned to `afterglow-labs/afterglow-cybersecurity` so it cannot replace this build with a ScopeWeaver release. No Afterglow release artifacts are assumed to exist yet.

## Capture browser traffic

1. Load [`extension/`](extension/) as an unpacked Chrome or Edge extension.
2. Sign in to the local Afterglow app. In its browser developer console, read `localStorage.getItem("artex_token")`, then paste that value and the Afterglow URL into the extension popup.
3. Open the authorized test tab, choose **Start capture**, browse, then choose **Stop**. The extension sends batches of at most 10 exchanges and retries queued entries when you choose **Flush now**. Unsent exchanges survive service worker restarts in session storage; if its 7 MiB queue fills, the oldest entries are dropped and counted as failed. The server accepts at most 50 exchanges per request and truncates each body at 256 KiB.
4. Review captured exchanges in **Traffic** and bind relevant entries to findings.

The extension sends its token only to HTTPS URLs or localhost over HTTP. Captured traffic can include session headers and response data, so protect the local traffic store and remove the extension token after the session if it is no longer needed.

## Defensive content and skills

[`detections/`](detections/) contains rules and a read-only host triage utility. Run `detections/tests/run-all.sh` on a machine with Docker to verify the full ruleset. The source, indicator, ATT&CK, and host triage checks can also run directly with Python; exact commands and their current status are in [merge decisions and verification](docs/AFTERGLOW-MERGE.md).

The additional playbooks in [`skills/`](skills/) are loaded through the existing skill system. They retain source attribution and an explicit local-only scope note. Grant agent visibility to the skills you intend to use.

## Provenance and license

This repository is a derivative of [ScopeWeaver](README.scopeweaver.md) and [ARTEX](https://github.com/Autumn-27/ARTEX), with credited material from the other local forks listed above. The application remains under [AGPL-3.0](LICENSE); the web frontend retains its separate [MIT license](web/LICENSE). Preserve each imported component's notices when redistributing. The original ScopeWeaver [provenance](docs/PROVENANCE.md) and [verification record](docs/VERIFICATION.md) describe the baseline, while [Afterglow's merge record](docs/AFTERGLOW-MERGE.md) describes this checkout.
