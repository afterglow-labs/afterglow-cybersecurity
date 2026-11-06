# Contributing to APTEX by Afterglow Labs

Afterglow combines code from several ARTEX-derived repositories. Keep changes focused, record their source when porting code, and retain AGPL-3.0 and component notices. The inherited ScopeWeaver process is saved in [CONTRIBUTING.scopeweaver.md](CONTRIBUTING.scopeweaver.md) for reference; this repository does not require its `dev` to `main` promotion route.

Maintain APTEX as an independent application. Ports from Odysseus belong in this checkout and use APTEX's server lifecycle, accounts, configuration, and data. Record the source and retained license notices for each port in [docs/AFTERGLOW-MERGE.md](docs/AFTERGLOW-MERGE.md).

Use locally isolated environments you own or are explicitly authorized to test. Task scope is guidance, so configure interception rules and network egress controls for any test environment. Do not add an external service dependency or a hardcoded public connector URL to a core workflow.

When changing the backend, run `go test ./... -count=1` with a disposable `ARTEX_TEST_DSN` for database-backed tests. When changing the frontend, run `npm ci`, `npm test`, and `npm run build:static` in `web/`. When changing the browser extension, run `node extension/background.test.mjs` and `node --check` on its scripts. Detection changes require `detections/tests/run-all.sh` with Docker. Record any unavailable check and its reason in the PR.

The UI ships English, Korean, Simplified Chinese, Traditional Chinese, Japanese, and Spanish. Update corresponding strings together. See the [locale contributor guide](web/src/i18n/README.md). Keep [docs/AFTERGLOW-MERGE.md](docs/AFTERGLOW-MERGE.md) aligned with imported code and verification evidence.
