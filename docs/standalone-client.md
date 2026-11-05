# Independent APTEX client and server

APTEX opens its full console in a dedicated app window with a separate browser
profile, login, and backend. The server normally listens on
`http://127.0.0.1:8787`; Odysseus runs independently on its configured port.

From the actual APTEX repository root:

```sh
test -f scripts/aptex.py
python3 scripts/aptex.py install
aptex-start
aptex
```

The installer adds `aptex`, `aptex-start`, and `aptex-stop` under
`~/.local/bin/`, plus an APTEX desktop entry. Commands resolve files from the
script's checkout. Paths containing spaces are supported. An existing managed
`odysseus-cyber-start` or `odysseus-cyber-stop` alias is redirected to APTEX.

| Command | Action |
| --- | --- |
| `aptex-start` | Prepare APTEX configuration, start Docker if needed, and start APTEX services |
| `aptex` | Open the full APTEX console in its own app window |
| `aptex-stop` | Stop only APTEX, PostgreSQL, and its private Ollama service |
| `aptex-start --rebuild` | Rebuild the APTEX image and apply the current source |

Start reuses running services. Stop retains database, account, and key volumes.
Each application's commands control its own server lifecycle. The existing
Docker project name `afterglow` is retained so the current data volumes remain
attached to APTEX.

## Login

The default APTEX username is **Afterglow**. Initial setup saves a generated
password in `~/.config/afterglow-security/afterglow-admin.txt`, readable only by
your user. Read it with:

```sh
cat ~/.config/afterglow-security/afterglow-admin.txt
```

Change the password in the console after signing in. The file records the
generated initial password, so later password changes are managed in APTEX.
An already initialized administrator is preserved. Odysseus keeps its own
accounts. The launcher does not mint an Odysseus gateway token.

## Local model

The native start command uses the existing local Ollama model, normally
`qwen3:32b`, with strict local LLM networking enabled. Set `OLLAMA_MODELS_DIR`
in this checkout's `.env` to the existing directory containing `blobs/` and
`manifests/`. The launcher also checks the usual host Ollama stores when this
setting is absent.

The model store is mounted read-only. Ollama stays on APTEX's private Docker
network with cloud disabled, and its current address is added to the LLM
allowlist. Starting the app does not download a model.

The client command requires a Chrome or Chromium browser and a healthy APTEX
server. Its profile lives at `~/.local/share/aptex/client/`. It does not start
Docker or either application server. `aptex --no-open` checks the client URL
without opening a window.
