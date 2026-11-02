---
name: rce-hunter
description: Hunt remote code execution on an authorized target — command injection (separators, time-based, blind), SSTI (Jinja2/Twig/Freemarker/Velocity/ERB detectors + object-graph walking), deserialization (Java/PHP/Python/.NET), file-write-to-exec. Invoke when an intent is about RCE, code execution, template injection, or unsafe deserialization.
---

> **Afterglow scope:** Use only in a locally isolated environment you own or are explicitly authorized to test. The local-only restriction takes precedence over any references below to online programs. Task scope is guidance; configure interception rules and network egress controls before testing.


# RCE Hunter

Authorized hunting only. Operate strictly inside the task's recorded scope.

## Doctrine (non-negotiable)

1. **Full corpus at every input point, no sampling, no early exit.** Every parameter that can reach an interpreter gets the full separator/engine matrix.
2. **Read before you probe.** Fingerprint first: server headers, error stacks, exception types, framework banners, uploaded-file echoes. The engine (Jinja2 vs Twig vs Shell) decides the whole corpus.
3. **Blind ≠ no evidence.** Time-based and OOB-free behavioral proof (status-code side channels, byte-length side channels, response timing) is valid evidence when the baseline variance is measured and the delta is ≥3σ over the baseline.
4. **One confirmed primitive is the start, not the end.** After any execution primitive: read `/etc/passwd` or env, confirm identity, then look for the second step (file write, config, credential access). Record the chain as a fact even before it is a finding.
5. **Classify every miss.** coverage-gap vs probe-defect, written into the fact.

## Workflow

### 1. Surface map (fingerprint first)
- Tech from headers + JS bundles + error pages: Python (Flask/Django/FastAPI), Java (Spring/Tomcat), PHP, .NET (ASP.NET), Node.
- Grep the JS bundles and recorded traffic for: template delimiters (`{{`, `${`, `<%=`, `#{`), command-shaped fields (`host`, `ip`, `cmd`, `file`, `path`, `template`, `name`), serialization content-types.
- Note every place user input can reach: query, path, headers (User-Agent, X-Forwarded-For, Referer — often forgotten), body fields, uploaded filenames.

### 2. Pick the attack family by surface
| Surface signal | Family |
|---|---|
| `{{`/`${`/`<%=` echoed, template engines in stack traces | SSTI → `references/ssti.md` |
| host/ip/ping-like params, cron/export tools, "check" endpoints | cmdi → `references/cmdi.md` |
| `.do`/`.action`/`/api/*` accepting `application/x-java-serialized-object`, or JSON with `@type`/`className` | Java deser → `references/deser.md` |
| PHP + `__construct`-shaped errors, or fields accepting base64/serialized blobs | PHP deser |
| `pickle`/`marshal` in stack traces, or ML/pipeline endpoints | Python deser |
| `.aspx` + `viewstate`/`__ViewState` | .NET deser |
| file upload / import / report generation | file-write→exec chain |

### 3. Probe (one tool call per family per point)
```bash
# command injection: full separator matrix, echo + time-based blind
python3 ${SKILL_DIR}/scripts/cmdi_probe.py --url 'http://host/check' --param 'ip'

# SSTI: engine detection matrix + object-graph walk
python3 ${SKILL_DIR}/scripts/ssti_probe.py --url 'http://host/render' --param 'tpl'
```
- Both scripts emit JSON: per-payload `status`, `reflected`, `exec_signal` (echo match / timing delta / error signature), `evidence`.
- **Echo first, blind second.** If any payload echoes its command output, you have a direct primitive — stop burning time-based rounds and go to confirmation (step 4).
- Header injection surface: re-run the winning separator through `User-Agent` / `X-Forwarded-For` / `Referer` — many filters only guard the query string.

### 4. Confirm the primitive (before reporting)
- Identity: `id` / `whoami` / `phpinfo()` / `System.getProperty("user.name")` / `Process.GetCurrentUser`.
- Scope: can you read files outside the app dir? Write to a writable path? (`/etc/passwd` read, or write a canary file to a known-writable location and fetch it.)
- Record: the exact winning request, the raw output, the baseline timing table (for blind). This is the PoC.

### 5. Second-step (the chain, always check)
- File write: temp/report/upload dir → webshell or cron or template dir.
- Config/env: `env`, `/proc/self/environ`, `config` object — keys, DB DSNs, API tokens → record as facts (these feed IDOR/SSRF chains).
- Internal network: can the primitive reach `169.254.169.254` / internal hosts? Record for the SSRF chain (do not exfiltrate; note the reachability as a fact).

### 6. Record
- Confirmed: `report_finding` with reproducible PoC request + raw output evidence; bind traffic refs (baseline + proof).
- Partial (primitive but unconfirmed scope): `record_fact` observed, summary "RCE primitive at <point>: <family>, identity <x>, writable <paths>".
- Negative: `record_fact` with the matrix counts and miss classification.

## Hard stops
- No destructive commands. No `rm`, `dd`, `fork bomb`, no data modification outside inert canaries.
- No outbound callbacks (no `curl attacker.com`). Proof is response-based or local-file-based.
- Time-based: baseline ≥5 samples, delta threshold ≥3σ, sleep ≤3s per probe.
