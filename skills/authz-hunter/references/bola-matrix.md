# BOLA / BFLA Matrix Reference

## Id-position matrix (where the object id lives)
| Position | Example | Swap method |
|---|---|---|
| path segment | `/api/orders/123` | replace the numeric/uuid segment |
| path multi-segment | `/api/users/7/orders/42` | replace the SECOND segment (the object), keep the owner |
| query | `?order_id=42` | replace value |
| JSON body | `{"order_id": 42}` | replace key value |
| JSON nested | `{"target": {"id": 42}}` | replace nested value |
| header | `X-Order-Id: 42` | replace header value |
| cookie | `selected=42` | replace cookie value |

**The object is the id that identifies the data, not the actor.** In
`/api/users/7/orders/42`, actor=7 (usually checked), object=42 (often NOT checked).
Matrix the object id, keep the actor fixed, then swap the actor too (2D matrix).

## Id-space rules
- Use **real ids from traffic** first (own, others', recently-created).
- Boundary set always: `0`, `-1`, `<max_seen>+1`, `<max_seen>+1000`, `99999999`.
- Type confusion: uuid endpoint → try `1`, `1;`, `1&2`, `{"$ne": null}` (NoSQL), `*`, empty string.
- Natural keys: email-as-id, username-as-id, slug-as-id — the "id" is a string; matrix = real foreign keys + `../../`-style on file-ish keys.

## The 2D matrix (actor × object)
For endpoint with actor a, object o:
```
              o=own   o=other   o=none   o=0    o=-1   o=max+1
actor=alice    .       FIND?     404?     404?   404?   404?
actor=bob      FIND?   .         404?     404?   404?   404?
actor=guest    FIND?   FIND?     404?     404?   404?   404?
```
- `actor=alice, o=other` 200+data → **BOLA** (object not bound to actor).
- `actor=guest` 200+data → **pre-auth BOLA** (P0/P1).
- `actor=bob, o=alice's` 200+data → cross-tenant BOLA.
- Any 200 where the response body contains **the object owner's data** (name/email/
  account fragment) = leak confirmed regardless of status.

## Response-shape triage (fast, no full body parse)
- status 200 + bytes ≈ own-object bytes → likely BOLA, parse for leak markers.
- status 200 + bytes ≈ 0/empty `{"data":null}` → id validated, authz maybe OK → try method variants.
- status 403 → control present → bypass variants (see below).
- status 404 → id space checked → the object id exists but is denied-by-absence → try uuid→int confusion, different position.

## 403 bypass variants (when a control fires)
1. method swap: GET→POST/PUT/PATCH/DELETE/OPTIONS on same path.
2. header spoof: `X-Forwarded-For: <own-ip>`, `X-Original-URL: /api/orders/<own>`, `X-Rewrite-URL:`, `X-Forwarded-Host:`.
3. path normalization: `//api/orders/`, `/api/orders/101/`, `/api/orders/101;`, `/api/orders/%31%30%31`, `/api/../api/orders/101`, `/API/orders/101` (case), trailing `?x=1` / `#frag`.
4. content-type swap on POST/PUT: form vs JSON vs multipart — different handler, sometimes different authz.
5. id encoding: `101` → `101.0`, `0x65`, `0101`, `+101`, `1e2` (numeric coercion bugs in some stacks).
6. batch endpoints: `?ids=101,102` or `{"ids":[101,102]}` — one allowed id in a batch can carry the denied one.
