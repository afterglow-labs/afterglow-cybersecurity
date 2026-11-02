---
name: authz-hunter
description: Hunt authorization flaws on an authorized target — BOLA/BAL full ID matrix, role boundaries, mass assignment, hidden admin APIs, object-merge gaps. Invoke when an intent is about access control, IDOR, broken object level authorization, or privilege escalation.
---

> **Afterglow scope:** Use only in a locally isolated environment you own or are explicitly authorized to test. The local-only restriction takes precedence over any references below to online programs. Task scope is guidance; configure interception rules and network egress controls before testing.


# Authz Hunter

Authorized hunting only. Operate strictly inside the task's recorded scope.

## Doctrine (non-negotiable)

1. **Full ID matrix, no sampling.** Every object-id-bearing endpoint gets the full matrix: own-id, other-user-id, nonexistent, boundary ids (0, -1, max, type-confused). One "tried one other id and got 403" is not a negative.
2. **Actors first.** You cannot test a boundary you haven't mapped. List the actors (roles, accounts, sessions) and which session/cookie belongs to which actor BEFORE replaying.
3. **The 403/404 is a data point, not a wall.** 403 = control exists (test bypass variants). 404 = the id space is validated (enumerate the real id space from traffic). 200 with empty data = the control is the query, not the auth (test other ids).
4. **Every role gets the replay.** A request valid for actor A is replayed as actor B, as guest (no session), and as any higher role — three calls, one judgment each.
5. **Classify every miss** (coverage-gap vs probe-defect) and write it into the fact.

## Workflow

### 1. Actor & session inventory
- From traffic + task context: who are the actors? (guest, user, moderator, admin, service accounts, API keys)
- For each actor, the credential handle: cookie jar / Authorization header / signed token.
- Record as a fact: `actors: user=alice(cookie A), bob=carol(cookie B), admin=...` with the session identifiers (never the secrets).
- If you need a second account and none exists in the recorded traffic: register one via the app's own signup flow (in scope), or note coverage-gap.

### 2. Object-id surface map
- `traffic_search` for requests with id-shaped values: path segments (`/api/orders/123`), query (`?user_id=`), JSON keys (`"id":`, `"order_id":`, `"account_id":`), headers.
- For each endpoint, note: id position, id type (int/uuid/natural key), and which actor created/owns the objects.
- Enumerate the **real id space**: collect the actual ids seen in traffic (own + others'). Do not guess 1..N unless the ids are small consecutive ints.

### 3. Full matrix (one tool call per endpoint)
```bash
python3 ${SKILL_DIR}/scripts/bola_matrix.py \
  --url 'http://host/api/orders/101' \
  --ids '101,102,999999,0,-1' \
  [--method POST] [--body '{"order_id": 101}'] \
  [--sessions 'alice=<cookie-A>; bob=<cookie-B>'] \
  [--guest] [--out /tmp/bola.json]
```
(The id must literally appear in the URL path, a query value, or a body field — the script auto-detects the position and swaps it per cell. Omit `--sessions` for the single-actor matrix — you still get the nonexistent/boundary columns. `--guest` adds a no-cookie actor.)
- Output JSON: per-cell `status`, `bytes`, `leak` (foreign data markers: other usernames/emails/phone fragments), `evidence`.
- **Leak beats status.** A 200 with actor B's data in actor A's session is the finding, even if the status code looks "normal".

### 4. Role-boundary replay
For each sensitive endpoint:
- Replay actor A's request with actor B's session (BOLA/BFLA).
- Replay as guest (no auth headers) — pre-auth reachability.
- Replay the **method variants**: `GET /admin/x` → also `POST`, `PUT`, `PATCH`, `DELETE` on the same path; JSON body variants of a GET endpoint (hidden methods).
- Check the response **shape** diff: 403 with an error body vs 404 — record which control fires (authn vs authz vs id-validation).

### 5. Mass assignment (on every create/update)
For each object CRUD endpoint, send the create/update body plus these added fields (one request, all fields — see `${SKILL_DIR}/references/mass-assign.md`):
`role`, `is_admin`, `admin`, `permissions`, `owner_id`, `user_id`, `balance`, `price`, `status`, `verified`, `active`, `bypass_2fa`.
- Signal: the response echoes the field with the injected value, or a later read-back shows it stuck.
- Read-back is the proof: create object → list/get it → the field persisted.

### 6. Hidden admin APIs
- Path matrix on the base routes: `/admin`, `/api/admin`, `/internal`, `/debug`, `/actuator`, `/graphql` (introspection), `/api/v2` variants of a known route.
- Parameter-gated: `?as=admin`, `?role=`, `X-Role:`, `X-User-Role:` headers on known endpoints (client-trust bugs).
- Record every 404-vs-403 distinction — the 403s are the live admin surface to keep hunting.

### 7. Record
- Confirmed: `report_finding` (BOLA/BAL/priv-esc) with the exact request/response pair per cell; bind traffic refs.
- Partial: `record_fact` observed (e.g. "role field accepted on /api/users — persisted, read-back confirmed") — these are P1 seeds even before a full exploit.
- Negative: `record_fact` with the matrix shape (n ids × n sessions) and miss classification.

## Hard stops
- Do not modify other users' data beyond what the matrix proves (read-back confirmations only; revert what you changed if trivially reversible).
- No brute-force of id spaces larger than the recorded real ids + boundary set (that is scanning, not hunting).
- Secrets found in foreign objects: record existence + shape, do not use them to pivot unless the task constraints allow.
