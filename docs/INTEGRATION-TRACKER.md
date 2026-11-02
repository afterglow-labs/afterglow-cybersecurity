# Afterglow integration tracker

The product goal is one application with the useful capabilities from ScopeWeaver, Artex-on-ChatGPT, artex-deep-hunter, artex-browser-chat, and KSIR-ARTEX. This tracks the integrated code, not a release claim.

| Capability | Current state | Release gate or intentional boundary |
| --- | --- | --- |
| ScopeWeaver task graph, agent routing, six-language UI, MCP, notifications, local LLM controls | Base | Full regression suite still needed. |
| ChatGPT OAuth, account storage, refresh/recheck/quota, Responses adapter, profile routing, model catalog, UI | Integrated | End-to-end login, refresh, replay, quota, and blocked-plan cases need a real account and full build. |
| Persistent browser sessions, operator control, screenshots and image compaction | Integrated | Browser tests need Playwright and an executable app. |
| Authenticated browser traffic extension | Integrated in `634e705` | Full browser capture test remains. |
| Workflow canvas, manual/interval/daily schedules | Integrated | Database and scheduler tests need PostgreSQL. |
| Operator questions, encrypted test accounts, task outcomes, lessons, skill gaps | Integrated | End-to-end task tests need the full app. |
| Deep Hunter follow-up waves, cross-task hints, campaign branches, hunt board, business map, attack chains and verification, stall watchdog | Integrated | Database and lifecycle tests need PostgreSQL. Chain composition is operator-started and verification is a separate task. |
| Native race, identity, and inflight request replay | Integrated with scope, capture, and approval controls | Full worker and proxy integration tests need PostgreSQL and a running capture proxy. |
| Defensive rules, ATT&CK layer, indicators, host triage | Integrated in `634e705` | Docker detection suite remains. |
| Proxy graph, artifact index, C2 handoff, cloud and lateral planning | Integrated with operator and scope controls | C2 is a handoff record; cloud/lateral tools do not claim to execute external actions. |
| Optional Telegram MCP helper | Integrated as stdio plugin | Operator must install Python dependencies, supply credentials and destination allowlist, and connect it manually. No HTTP wizard. |
| Additional source playbooks | 29 imported; embedded credentials removed and seven risky playbooks rewritten | Runtime playbooks remain opt-in. Executable source payloads and WAF mutation lists live only in the archived source-assets directory. |
| KSIR mocked offensive success tools | Not enabled as execution | Their source implementations returned success without performing the claimed action. Handoff, plans, and local artifact indexing replace those claims. |

## ChatGPT account path

The plan profile stores only an account ID in PostgreSQL. The private OAuth store has a dedicated Docker volume. Plan inference uses the official public Responses endpoint and does not enter API-key failover. Opaque replay state stays in the transcript and is removed from debug request records. `SCOPEWEAVER_LOCAL_LLM_ONLY=true` blocks plan inference and the account model catalog.

## Verification limits

Pure Go package tests, Telegram Python syntax, translation parity, and the initial extension/detection checks pass. Full Go and Next.js builds are blocked by missing cached dependencies and unavailable network access. The formatter test requires `date-fns`. A complete run still needs PostgreSQL, Docker, and a browser; see [merge verification](AFTERGLOW-MERGE.md).
