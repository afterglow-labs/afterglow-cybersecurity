#!/usr/bin/env python3
"""bola_matrix.py — full actor × object-id matrix in one Bash call. JSON out.

Stdlib only. Honors HTTP(S)_PROXY / ALL_PROXY env and trusts SSL_CERT_FILE.

Usage:
  python3 bola_matrix.py --url 'http://host/api/orders/101' \
      --id-field 'order_id' --ids '101,102,999999,0,-1' \
      --sessions 'alice=<cookie-A>; bob=<cookie-B>' [--as guest]
      [--method GET] [--body '{"order_id":101}'] [--header 'X: y'] [--out /tmp/b.json]

The id is swapped in path / query / body (whichever matches --ids first, or
--id-field in body/query). Each cell reports status + bytes + a body snippet;
leak judgment is the model's call, but identical-content grouping is precomputed.
"""
import argparse
import concurrent.futures as cf
import hashlib
import json
import os
import re
import ssl
import urllib.error
import urllib.parse
import urllib.request

TIMEOUT = 15
MAX_BODY = 200_000


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


def swap_id(url, body, ids, field):
    """Return (url, body, position) with the FIRST matching id replaced by placeholder __ID__."""
    q = urllib.parse.urlsplit(url)
    # 1) path: last segment that matches one of the ids
    parts = q.path.split("/")
    for i in range(len(parts) - 1, -1, -1):
        if parts[i] in ids:
            parts[i] = "__ID__"
            new = urllib.parse.urlunsplit((q.scheme, q.netloc, "/".join(parts), q.query, q.fragment))
            return new, body, "path"
    # 2) query
    if q.query:
        pairs = urllib.parse.parse_qsl(q.query, keep_blank_values=True)
        for i, (k, v) in enumerate(pairs):
            if v in ids or (field and k == field):
                pairs[i] = (k, "__ID__")
                new = urllib.parse.urlunsplit((q.scheme, q.netloc, q.path,
                                               urllib.parse.urlencode(pairs, doseq=True), q.fragment))
                return new, body, "query:%s" % k
    # 3) body
    if body is not None:
        try:
            doc = json.loads(body)
            if _find_id(doc, ids, field):
                return url, json.dumps(doc, ensure_ascii=False), "body"
        except Exception:
            pairs = urllib.parse.parse_qs(body, keep_blank_values=True)
            for k, vals in list(pairs.items()):
                if field and k == field or any(v in ids for v in vals):
                    pairs[k] = ["__ID__"]
                    return url, urllib.parse.urlencode(pairs, doseq=True), "body:%s" % k
    return url, body, "none"


def _find_id(doc, ids, field):
    if isinstance(doc, dict):
        for k, v in doc.items():
            if (field and k == field) or (isinstance(v, (str, int)) and str(v) in ids):
                doc[k] = "__ID__"
                return True
            if _find_id(v, ids, field):
                return True
    elif isinstance(doc, list):
        for v in doc:
            if _find_id(v, ids, field):
                return True
    return False


def clip(text, n=220):
    return text[:n]


def run(url, method, body, headers, ids, sessions, as_guest, ctx):
    turl, tbody, position = swap_id(url, body, ids, None)
    if position == "none":
        return {"error": "no injectable id position found — pass a URL/query/body where one of --ids appears"}

    actors = list(sessions.items())
    if not actors and not as_guest:
        actors.append(("default", ""))
    if as_guest:
        actors.append(("guest", ""))

    cells = []

    def fire(actor, cookie, ident):
        u = turl.replace("__ID__", ident)
        b = tbody.replace("__ID__", ident) if tbody else tbody
        hs = list(headers)
        if cookie:
            hs.append(("Cookie", cookie))
        status, raw, err = do_request(u, method, hs, b, ctx)
        text = raw.decode("utf-8", "replace") if raw else ""
        digest = hashlib.sha256(raw).hexdigest()[:12] if raw else None
        return {
            "actor": actor, "id": ident, "status": status,
            "bytes": len(raw) if raw else 0, "digest": digest,
            "snippet": clip(text), "error": err,
        }

    with cf.ThreadPoolExecutor(max_workers=4) as ex:
        futs = []
        for actor, cookie in actors:
            for ident in ids:
                futs.append((actor, ident, ex.submit(fire, actor, cookie, ident)))
        for actor, ident, f in futs:
            cells.append(f.result())

    # group identical responses across cells (same digest, >1 cell, mixed actors
    # or mixed ids) — the classic shared-object signature
    by_digest = {}
    for c in cells:
        if c["digest"]:
            by_digest.setdefault(c["digest"], []).append(c)
    shared = []
    for d, group in by_digest.items():
        if len(group) > 1:
            actors_set = {g["actor"] for g in group}
            ids_set = {g["id"] for g in group}
            if len(actors_set) > 1:
                shared.append({"digest": d, "actors": sorted(actors_set),
                               "ids": sorted(ids_set), "status": group[0]["status"],
                               "snippet": clip(group[0]["snippet"] or "", 160)})

    n200 = sum(1 for c in cells if c["status"] == 200)
    verdict = "MATRIX_COMPLETE"
    if shared:
        verdict = "SHARED_RESPONSE_CANDIDATE"
    return {
        "url_template": turl.replace("__ID__", "<ID>"), "id_position": position,
        "actors": [a for a, _ in actors], "ids": ids,
        "verdict": verdict, "n_cells": len(cells), "n_200": n200,
        "shared_response_groups": shared,
        "cells": cells,
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--method", default="GET")
    ap.add_argument("--body", default=None)
    ap.add_argument("--header", action="append", default=[])
    ap.add_argument("--ids", required=True, help="comma-separated id list")
    ap.add_argument("--sessions", default="", help="name=<cookie>; name2=<cookie2>")
    ap.add_argument("--guest", dest="as_guest", action="store_true",
                    help="include a guest (no cookie) actor")
    ap.add_argument("--out", default=None)
    a = ap.parse_args()

    ids = [x.strip() for x in a.ids.split(",") if x.strip()]
    sessions = {}
    for part in a.sessions.split(";"):
        part = part.strip()
        if "=" in part:
            k, v = part.split("=", 1)
            sessions[k.strip()] = v.strip()

    headers = []
    for h in a.header:
        if ":" in h:
            k, v = h.split(":", 1)
            headers.append((k.strip(), v.strip()))
    if a.body and not any(k.lower() == "content-type" for k, _ in headers):
        headers.append(("Content-Type", "application/json"))

    out = run(a.url, a.method.upper(), a.body, headers, ids, sessions, a.as_guest, ctx_ssl())
    s = json.dumps(out, ensure_ascii=False, indent=1)
    print(s)
    if a.out:
        with open(a.out, "w", encoding="utf-8") as f:
            f.write(s)


if __name__ == "__main__":
    main()
