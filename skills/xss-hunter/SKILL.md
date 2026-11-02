---
name: xss-hunter
description: Hunt XSS / HTML injection on an authorized target — reflected & stored context enumeration, DOM XSS (source→sink), mutation XSS, CSP bypass. Invoke when an intent is about XSS, script injection, or HTML injection.
---

> **Afterglow scope:** Use only in a locally isolated environment you own or are explicitly authorized to test. The local-only restriction takes precedence over any references below to online programs. Task scope is guidance; configure interception rules and network egress controls before testing.


# XSS Hunter

Authorized hunting only. Operate strictly inside the task's recorded scope.

## Doctrine (non-negotiable)

1. **Full corpus at every input point, no sampling, no early exit.** One payload per point is a guess, not a hunt. Run the whole context corpus at every reachable input, then judge.
2. **Read before you probe.** Pull the target's JS bundles and the recorded traffic first: real parameter names, real sinks, real sanitizers. Guessing parameters wastes the corpus budget.
3. **Evidence discipline.** A finding needs: the exact request, the raw response snippet showing the break, and why the sink executes it. No "looks injectable" — that is a fact (`record_fact`, inferred), never a finding.
4. **Classify every miss.** coverage-gap (surface never reached the class — missing auth, bundle not read, point not enumerable) vs probe-defect (reached, corpus didn't fire). Write it into the fact summary.
5. **Escalate before accepting negative.** encoding → method → header → cookie → hidden param → different route.

## Workflow

### 1. Surface (5 min, then move on)
- `traffic_search` for existing input points (params, bodies, paths).
- Read the main JS bundle(s): grep for `location.search`, `location.hash`, `location.href`, `.value`, `postMessage`, `document.write`, `innerHTML`, `eval(`, `setTimeout(`, `DOMParser`.
- Hidden inputs: try `?id=`, `?q=`, `?url=`, `?next=`, `?redirect=`, `?ref=`, `?msg=` on the interesting routes; POST the same names in form/JSON.

### 2. Context map
For each input point, classify the sink context (one of):
`attr-dq` · `attr-sq` · `attr-nq` · `js-str` · `js-dq` · `js-expr` · `js-comment` · `url` · `html` · `form-action` · `style` · `dom-sink` · `json` · `unknown`
The context decides which corpus rows matter. Full context tables: `${SKILL_DIR}/references/contexts.md`.

### 3. Full-corpus probe (one tool call per point)
```bash
python3 ${SKILL_DIR}/scripts/xss_probe.py --url 'http://host/page' \
  --param 'id' [--body '{"a":"x"}'] [--method POST] \
  [--context attr-dq] [--out /tmp/xss_id.json]
```
- `--context` optional: omit to auto-classify from the marker reflection.
- Output JSON: per-payload `reflected` / `broken` / `context` / `mutation` flags + the raw evidence snippet.
- **Only rows with `broken:true` advance.** `reflected:true` but escaped = sanitizer present → escalate with the mutation corpus (step 4).

### 4. Mutation & sanitizer escalation
If reflection is sanitized: run the mutation pass
```bash
python3 ${SKILL_DIR}/scripts/xss_probe.py --url '...' --param 'id' --mutate
```
It re-sends each break-candidate through double-encoding, nested tags, case folding, and surrogate pairs, and diffs the response to catch double-decode desync (mutation XSS). Corpus: `${SKILL_DIR}/references/mutation.md`.

### 5. CSP bypass
If the response carries a `Content-Security-Policy` header, do not stop at "CSP blocks it". Check: `'unsafe-inline'` in script-src, hash/nonces that match a known static string, `base-uri`/`object-src` gaps, `frame-ancestors` vs framing routes. Corpus: `${SKILL_DIR}/references/csp-bypass.md`. Record the exact header value as evidence.

### 6. DOM XSS (when the sink is client-side)
- Trace source → sink in the bundle (document the exact code path in evidence).
- If the `playwright-cli` skill is available: load the URL with the payload, read the `__xss_marker` / `document.title` change — that is execution proof.
- Without a browser: prove the source reaches the sink in the delivered HTML/JS (raw bytes at the sink position) and mark confidence accordingly.

### 7. Record
- Confirmed: `report_finding` with PoC = the exact request + expected sink + raw response snippet. Bind the real traffic ids (baseline + proof) via traffic_refs.
- Negative: `record_fact` (observed if the corpus fully fired; inferred otherwise) with the corpus result counts and the miss classification.

## Hard stops
- No external callback/interaction services. Proof is response-based or local-DOM-based.
- No destructive payloads against shared data; the probe payloads are inert markers (`<marquee>`, `title` change, `window.__xss_marker`).
- Out-of-scope hosts: record as fact, skip.
