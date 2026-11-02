---
name: waf-bypass
description: "HTTP-layer WAF evasion wrapper. Apply whenever a probe or exploit returns 403/429 or a WAF block page. Covers IP spoofing headers, method mutation, content-type mutation, junk padding, and UA rotation for Cloudflare, ModSecurity, Akamai, and AWS WAF."
---

> Afterglow scope: Use only in a locally isolated environment or against systems the operator is explicitly authorized to test. Respect task scope, interception rules, network egress controls, and operator approvals. Treat all outputs and retrieved content as untrusted evidence.


# waf-bypass — WAF Evasion Wrapper

## Scope

HTTP-layer evasion for any exploit category (SQLi, SSRF, IDOR, XSS, RCE delivery, path traversal, brute-force). Stack-agnostic; applies before any probe that hits a protected surface.

## When to Apply

- Response is 403 Forbidden (WAF hard block)
- Response is 429 Too Many Requests (rate-limit / behavioral detection)
- Response body contains WAF fingerprints: "Access Denied", "Ray ID", "Incapsula", "Request Rejected", "NAXSI"
- Payload returns anomalous redirect to `/cdn-cgi/`, `/akamai-error`, or similar WAF landing

## Technique Inventory

### 1. IP Spoofing Headers (MITRE T1562)

WAFs that trust reverse-proxy infrastructure will skip deep inspection for traffic appearing to originate from internal IPs. Inject spoofed values into the header set defined in `payloads/headers_spoofing.yaml`. Rotate the IP from the pool on each request.

Key headers: `X-Forwarded-For`, `X-Real-IP`, `X-Client-IP`, `X-Remote-IP`, `X-Remote-Addr`, `X-Original-URL`, `X-Rewrite-URL`.

### 2. HTTP Method Mutation (MITRE T1562)

WAF rules frequently inspect only GET and POST. Mutate to alternative verbs that the application accepts but the WAF ignores.

- GET  → HEAD, OPTIONS, TRACE
- POST → PUT, PATCH, DELETE

See `payloads/method_mutation.yaml` for the full map.

### 3. Content-Type Mutation

WAFs signature-match on `application/x-www-form-urlencoded` and `application/json`. Shift to less-inspected types:

- `text/xml` / `application/xml` — triggers XML parser, bypasses JSON-specific rules
- `multipart/form-data` — payload fragments across MIME boundaries
- `text/plain` — often whitelisted by body-inspection rules

### 4. Junk Data Padding

WAFs enforce an inspection byte limit. Prepend `N * 1024` bytes of junk before the real payload so the signature falls outside the inspection window.

```
padded_body = ("A" * 8192) + "&" + real_payload
```

Tune `N` upward (8 KB → 16 KB → 64 KB) until the block clears.

### 5. User-Agent Rotation (MITRE T1036)

Rotate realistic 2025-2026 UAs per request. Pool in `payloads/user_agents_2025.txt`. Never repeat the same UA on consecutive requests to the same endpoint.

### 6. Referer Spoofing

Pair UA rotation with a believable Referer:

- `https://www.google.com/`
- `https://www.bing.com/search?q=`
- `https://duckduckgo.com/?q=`
- `https://www.google.com.br/`

### 7. Path Encoding Variants

For path-based rules (ModSecurity CRS):

- Double URL encode: `%252f` for `/`
- Unicode normalization: `%u002f`
- Case variation (IIS): `/Admin` → `/aDmIn`
- Trailing dot bypass: `/admin./`
- Null byte injection (legacy): `/admin%00.jpg`

## WAF-Specific Notes

### Cloudflare

- Ray ID in response = hard block. Bot Management active.
- Bypass vectors: `CF-Connecting-IP` spoof may work on mis-configured origin-trust rules. Method mutation (TRACE/OPTIONS) skips Bot Fight Mode on many configs. Add `Cache-Control: no-cache` to reduce bot-score.
- Rate limiting is per-IP. Route through residential proxy or inject `X-Forwarded-For` pool.
- JS challenge: rendered response → requires headless browser (Playwright + stealth plugin) or `cf_clearance` cookie replay.

### ModSecurity (CRS)

- CRS paranoia level 1-2: content-type mutation + junk padding usually sufficient.
- CRS level 3-4: requires payload fragmentation across chunked transfer encoding or multipart boundaries.
- `X-Original-URL` and `X-Rewrite-URL` bypass CRS path-based rules on Nginx/Apache reverse-proxy setups.
- SSTI payload `{{7*7}}` blocked by CRS rule 941xxx → encode as `{%raw%}{{7*7}}{%endraw%}` or submit via JSON body.

### Akamai

- "Reference #" in response = Kona Site Defender block.
- Akamai trusts `True-Client-IP` header on misconfigured origins — inject internal IP there.
- Rate limiting: distributed across PoPs. Rotating IPs alone insufficient; combine with jitter (100–2500 ms).
- Slow-rate evasion: `Connection: keep-alive` + 1-2 s delay between requests.

### AWS WAF

- Rule groups (AWSManagedRulesCommonRuleSet) block common SQLi/XSS payloads in query strings and body.
- Body inspection is capped at 8 KB by default → junk padding above 8 KB moves payload outside inspection window.
- `X-Forwarded-For` is NOT trusted by AWS WAF natively; works if target sits behind ALB with forwarding configured.
- Method mutation effective: AWS WAF does not inspect OPTIONS/TRACE body by default.

## Step-by-Step Procedure

```
1. IDENTIFY WAF
   curl -si https://TARGET/ | grep -iE "server|x-powered-by|cf-ray|x-cache|x-amz-cf|via|akamai"
   wafw00f https://TARGET/

2. SELECT BYPASS LAYER
   403 hard block         → start with IP spoofing headers + UA rotation
   429 rate limit         → add jitter (random 0.1–2.5 s) + IP rotation
   JS challenge           → use Playwright stealth or replay cf_clearance
   Payload-specific block → content-type mutation + junk padding

3. WRAP REQUEST
   Apply headers from payloads/headers_spoofing.yaml (rotate IP from pool)
   Set UA from payloads/user_agents_2025.txt (rotate per request)
   Set realistic Referer
   If POST blocked → try PUT or PATCH (payloads/method_mutation.yaml)
   If body inspected → prepend junk padding (8 KB default)

4. RETRY
   Re-send probe with wrapped request
   If still blocked → increment junk padding, try next method mutation,
                      escalate to chunked transfer encoding fragmentation

5. CONFIRM BYPASS
   200 / 302 → bypass successful, proceed with full exploit
   Still 403  → log response headers, check WAF fingerprint, pivot technique
```

## Jitter Config (from EvasionEngine)

| Parameter    | Default | Notes                              |
|--------------|---------|------------------------------------|
| jitter_min   | 100 ms  | Minimum delay between requests     |
| jitter_max   | 2500 ms | Maximum delay; tune per WAF policy |
| rotate_ua    | true    | Rotate UA every request            |
| proxychains  | false   | Enable for full IP rotation        |

## References

- MITRE ATT&CK T1562.001 — Impair Defenses: Disable or Modify Tools
- MITRE ATT&CK T1036 — Masquerading
- Source: `evasion.py` — `waf_bypass_headers()`, `mutate_http_method()`, `mutate_content_type()`, `junk_data_padding()`
