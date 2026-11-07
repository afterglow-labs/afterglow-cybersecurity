# APTEX by Afterglow Labs

Autonomous Purple Team Execution.

APTEX by Afterglow Labs combines the main capabilities of five ARTEX-derived projects in one self-hosted security research console. ScopeWeaver supplies the application base; the other forks contribute ChatGPT plan access, browser sessions, workflows, deeper hunting, defensive content, proxy controls, and operator-directed assessment tools. The application uses a Go backend, a Next.js frontend, PostgreSQL, a shared asset graph, and per-task exploration graphs. Use it only in locally isolated or explicitly authorized research environments.

## What is included

| Source | Integrated capability |
| --- | --- |
| ScopeWeaver `a89795a` | Core application: planner and workers, asset and evidence graphs, task controls, live work steering, MCP connections, scheduled agent triggers, local and hosted model profiles, six UI languages, and the agent adapter. |
| Artex-on-ChatGPT `afcd6ca` | ChatGPT account connection, OAuth refresh and quota checks, Responses inference, per-profile model selection, protected account storage, persistent browser sessions, and image input. |
| artex-browser-chat `13fabbb` | Authenticated browser traffic capture, workflow canvas and schedules, operator questions, task test accounts, outcomes, lessons and skill gaps, and an optional Telegram stdio MCP helper. |
| artex-deep-hunter `abbe8e4` | Bounded follow-up waves, cross-task finding hints, campaign branch board, business map and attack-chain hypotheses, verification tasks, scoped native request replay, stall watchdog, defensive rules, indicators, host triage, and focused hunting skills. |
| KSIR-ARTEX `240ae7c` | Proxy graph rotation, local artifact manifest, operator-controlled C2 handoff records, scoped cloud and lateral assessment plans, and additional playbooks. |

The source forks overlap and some source tools reported success without executing anything. Afterglow keeps the feature surface while making those paths explicit: cloud and lateral tools produce scoped assessment plans, `exfil_data` indexes real local artifacts without transferring bytes, and C2 tools create a disabled-by-default handoff for an operator. See [integration status](docs/INTEGRATION-TRACKER.md) and [merge decisions and verification](docs/AFTERGLOW-MERGE.md) for details and test limits.

The source fork's executable credential loader, AMSI bypass, PHP web shells, bulk exploit scripts, and WAF mutation lists are preserved in [source assets](extras/source-assets/README.md) for review. They are outside the runtime skills directory and excluded from the Docker build context. The active playbooks cover scoped assessment and evidence collection.

## Start locally

APTEX has its own client and server. From this repository root, install the
native shortcuts once, start its services, and open its dedicated app window:

```sh
python3 scripts/aptex.py install
aptex-start
aptex
```

`aptex-stop` stops only APTEX, PostgreSQL, and its private Ollama service while
retaining their data. Odysseus uses its own `odysseus-start`, `odysseus`, and
`odysseus-stop` commands. The [standalone client guide](docs/standalone-client.md)
covers login, the existing local model, configuration, and image rebuilds.

Remote devices can use the independent [Mac client](clients/macos/README.md) or
[native iPhone/iPad client](clients/ios/README.md). Each connects to the desktop
server without installing a server or model runtime on that device.

For a manual Compose deployment:

Docker and Docker Compose are required for the standard installation. Copy `.env.example` to `.env`, set `POSTGRES_PASSWORD`, then build this checkout:

```sh
cp .env.example .env
docker compose up -d --build
```

Open `http://localhost:8787` and create the administrator password at `/setup`. Compose binds the app to loopback by default. Configure a model in **System → LLM** before starting an agent task. The Docker service and executable still use the internal `scopeweaver` name, and environment variables retain `SCOPEWEAVER_*` and `ARTEX_*` names for compatibility with the base. `docker compose logs -f scopeweaver` shows startup logs.

The administrator username defaults to **Afterglow**. Set `AFTERGLOW_ADMIN_USERNAME`
in `.env` to use a personal name on the login screen; **Afterglow** remains an
alias. ARTEX is not a valid login name. Changing the
configured name invalidates existing administrator sessions.

If a local Ollama model is already downloaded on the Docker host, the optional
[`docker-compose.host-model.yml`](docker-compose.host-model.yml) runs Ollama on
the private Compose network with that existing model directory mounted read-only.
Export `OLLAMA_MODELS_DIR` as the directory that actually contains the model's
`blobs/` and `manifests/`, verify it exists, then start the `local-llm` profile:

```sh
test -d "$OLLAMA_MODELS_DIR"
docker compose -f docker-compose.yml -f docker-compose.host-model.yml --profile local-llm up -d --build
```

Use `http://ollama:11434` as the model endpoint inside ScopeWeaver. The Ollama
port is not published on the host, and the base Compose service disables Ollama
cloud. With `SCOPEWEAVER_LOCAL_LLM_ONLY=true`, allowlist the Ollama container's
actual `/32` address as described in the [local model network policy](docs/llm-providers.md#network-policy-local_llm_only). Container addresses can change when Compose recreates services.

## Optional integration API

APTEX retains its [scoped gateway token API](docs/gateway-api.md) for custom
integrations. Its independent client uses its own administrator session;
running APTEX does not configure credentials on Odysseus.

## Connect ChatGPT

Open **System → LLM**, press **Sign in with ChatGPT**, finish the browser sign-in, then create or select a `ChatGPT plan` profile. No API key is required for this account flow. The account connection and administrator login are separate. Docker stores account tokens in the `chatgpt_auth` volume; the database stores an account reference. The callback is fixed to `http://127.0.0.1:${ARTEX_PORT:-8787}/auth/callback`. When `SCOPEWEAVER_LOCAL_LLM_ONLY=true`, plan inference and the account model catalog are disabled. Set it to `false` and apply the configuration with `aptex-start` to use remote models; the launcher preserves this choice and keeps the local model available. A disconnected or blocked plan profile fails closed instead of switching to an API-key profile.

Both account and API-key profiles offer GPT-6 Astra, GPT-6.1 Sol, GPT-6 Sol, GPT-5.6 Sol, and Daybreak Blue Latest, alongside models returned by the provider. The Blue controls apply to GPT-6 Sol, GPT-5.6 Sol, and `gpt-daybreak-blue-latest`; Astra and GPT-6.1 Sol explicitly use standard access for a Blue-only installation. A Blue selection persists with the profile and sends `access_programs.cyber=daybreak_blue` without changing the selected model. These choices do not grant account or project access; OpenAI checks authorization on each request. See the [Daybreak request documentation](https://developers.openai.com/api/docs/guides/daybreak).

Plan use follows the account's available models and limits. [OpenAI's Sign in with ChatGPT documentation](https://developers.openai.com/siwc/token-sharing-open-source/sign-in) describes the account connection; [OpenAI's plan guidance](https://help.openai.com/en/articles/20001542-using-your-chatgpt-plan-in-other-apps-and-sites) explains plan availability in other apps.

## Workflows and task intelligence

**Workflows** provides a visual graph for manual, interval, and daily agent runs. The task **Learning** tab shows campaign branches, an optional hunt-class coverage board, a business map, and proposed attack chains. Operators can compose a chain from confirmed findings and launch a separate verification task. **System → Settings** controls bounded follow-up waves. The task view also includes test accounts, artifact metadata, and operator C2 handoff status where configured.

Workers also have `race_probe`, `session_store`, `identity_switch`, `identity_matrix`, and `inflight_modify` for authorized testing. Request replay requires an operator-entered task scope, a passing asset interception gate, and active traffic capture. Each replay call goes through the interception review, defaults to operator approval, and is bounded in request count, size, and time. Redirects and direct-network fallback are disabled.
The capture proxy verifies target TLS certificates by default. For an isolated lab with self-signed target certificates, set `ARTEX_TRAFFIC_INSECURE_UPSTREAM=true` explicitly.

The optional [Telegram MCP helper](plugins/telegram-mcp/README.md) requires operator-supplied credentials and an explicit destination allowlist. It does not start with the application.

For the optional C2 **handoff record** feature, place an operator-owned binary under `private/c2-binaries/`, set `ARTEX_ENABLE_C2_HANDOFF=true`, and point `ARTEX_C2_BIN_X64` or `ARTEX_C2_BIN_X86` to its `/app/c2-binaries/...` path in `.env`. Docker mounts that directory read-only and excludes it from the image build context and Git. The tool verifies its hash and creates a review record; it does not deliver the file.

For other installation modes and detailed operation, consult the [ScopeWeaver baseline guide](README.scopeweaver.md). Its release links and one-click updater refer to the original project; the updater in this checkout is pinned to `afterglow-labs/afterglow-cybersecurity` so it cannot replace this build with a ScopeWeaver release. No Afterglow release artifacts are assumed to exist yet.

## Capture browser traffic

1. Load [`extension/`](extension/) as an unpacked Chrome or Edge extension.
2. Sign in to the local Afterglow app. In its browser developer console, read `localStorage.getItem("afterglow_token")`, then paste that value and the Afterglow URL into the extension popup.
3. Open the authorized test tab, choose **Start capture**, browse, then choose **Stop**. The extension sends batches of at most 10 exchanges and retries queued entries when you choose **Flush now**. Unsent exchanges survive service worker restarts in session storage; if its 7 MiB queue fills, the oldest entries are dropped and counted as failed. The server accepts at most 50 exchanges per request and truncates each body at 256 KiB.
4. Review captured exchanges in **Traffic** and bind relevant entries to findings.

The extension sends its token only to HTTPS URLs or localhost over HTTP. Captured traffic can include session headers and response data, so protect the local traffic store and remove the extension token after the session if it is no longer needed.

## Defensive content and skills

[`detections/`](detections/) contains rules and a read-only host triage utility. Run `detections/tests/run-all.sh` on a machine with Docker to verify the full ruleset. The source, indicator, ATT&CK, and host triage checks can also run directly with Python; exact commands and their current status are in [merge decisions and verification](docs/AFTERGLOW-MERGE.md).

The additional playbooks in [`skills/`](skills/) are loaded through the existing skill system. They retain source attribution and an explicit local-only scope note. Grant agent visibility to the skills you intend to use.

## Provenance and license

This repository is a derivative of [ScopeWeaver](README.scopeweaver.md) and [ARTEX](https://github.com/Autumn-27/ARTEX), with credited material from the other local forks listed above. The application remains under [AGPL-3.0](LICENSE); the web frontend retains its separate [MIT license](web/LICENSE). Preserve each imported component's notices when redistributing. The original ScopeWeaver [provenance](docs/PROVENANCE.md) and [verification record](docs/VERIFICATION.md) describe the baseline, while [Afterglow's merge record](docs/AFTERGLOW-MERGE.md) describes this checkout.
