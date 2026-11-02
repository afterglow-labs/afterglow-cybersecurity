# Mutation XSS Corpus

Goal: break sanitizers that decode **twice**, or that decode in a different order than the browser.
Each mutation row is the base break payload passed through one transform; the probe diffs the
reflected bytes to detect desync.

## Transforms (applied to the base break for the point's context)
| id | transform | catches |
|---|---|---|
| d1 | percent-encode the whole payload once | server decodes once, browser sees raw |
| d2 | percent-encode twice | server decodes twice (classic double-decode) |
| d3 | `+` for space | URL-decode desync |
| d4 | unicode escape in JS string (`\u003c` style) | JS-level decode |
| d5 | nested tag `<scr<script>ipt>` | filter that strips known tags once |
| d6 | case fold `<ScRiPt>` | case-sensitive filter |
| d7 | split event `oNmouseOver` | case/word-boundary filter |
| d8 | tab instead of space in attribute | whitespace-class filter |
| d9 | `%00` / surrogate pair `&#x00;` mid-tag | entity edge cases |
| d10 | attribute split `" onmouseover = "x` | tokenizer desync |

## Desync detection
The probe sends `BASE`, then each mutation, and compares the **reflected bytes** against the
expected post-decode form. A hit means:
- the raw response contains the marker **outside** the quoted/escaped position, OR
- the same input reflects differently across two decodes (the response for d2 contains the
  unescaped form while the response for d1 shows one layer still encoded).

## Classic desync pairs (always test both sides)
- `<` as `&lt;` in HTML but sent as `%3C` to a JS handler that `decodeURIComponent`s before `document.write`.
- `</script>` escaped as `<\/script>` in JSON: if the app emits the JSON inside `<script>`
  without the slash-escape, `</script>` breaks out — send `"</script><marquee onstart=\"__xss_marker=1\">"`.
- HTML-entity vs URL-entity: `&#x22;` vs `%22` round-trips.
- Cookie-based: some apps decode cookies once in the header parser and again in the app.
  Pass the payload as `--header 'Cookie: probe=PAYLOAD'` and diff against the param reflection.
