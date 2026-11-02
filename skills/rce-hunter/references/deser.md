# Deserialization Reference

## Endpoint identification
| Signal | Likelihood |
|---|---|
| `Content-Type: application/x-java-serialized-object` accepted (not 415) | Java deser surface |
| `.do` / `.action` / `struts` in stack traces | Java (Struts2 — OGNL + deser) |
| JSON body with `@type` / `className` / `class` / `@class` keys echoed | Fastjson / Jackson polymorphic / Gadget |
| `viewstate` / `__VIEWSTATE` in .aspx | .NET ViewState (BinaryFormatter / LosFormatter) |
| `PHP_SESSION` / `serialize(` in errors | PHP |
| `pickle` / `UnpicklingError` in errors | Python |
| `/api/*` accepting `text/serialized`, `application/octet-stream` bodies | language-agnostic surface |

## Classpath / dependency discovery (before crafting gadgets)
Gadget chains need the right libraries on the classpath. Discover, don't guess:
1. Error stacks: exception class names reveal framework + version (`org.apache.struts2`, `com.fasterxml.jackson`, `org.springframework`).
2. `GET /actuator`, `/actuator/env`, `/actuator/mappings` (Spring Boot — often open pre-auth).
3. Exception-triggered stack dumps: send malformed serialized blobs, read the exception text.
4. JS bundles: API version constants.
5. JAR/WAR listing: `/.git/`, `/webapp/*.jar`, error pages naming libraries.
6. Response headers: `X-Powered-By`, `Server`, `X-AspNet-Version`.

Record the dependency list as a fact — it determines which chains are live.

## Java chains (by dependency)
| Dependency seen | Chain family |
|---|---|
| commons-collections | CC1–CC7 (ysoserial `CommonsCollections1..7`) |
| commons-beanutils + commons-collections | CB1 |
| spring-core | SpringAOP1 |
| groovy | Groovy1 |
| ibm jdk | URLDNS (blind confirm) |
| none confirmed | **URLDNS** first — blind RCE confirmation without gadgets |

URLDNS probe: serialized `java.net.URL` with hostname `canary.<taskid>.<oob-domain-if-allowed>` —
if OOB is forbidden, use `http://<in-scope-internal-host>:<port>` and check server-side
connection logs / error text ("UnknownHostException" vs "Connection refused" = it resolved
and tried to connect = deserialization is live).

## PHP
- Detect: `unserialize()` in stack trace, `Warning: ... serialize()` errors.
- Magic methods: `__wakeup`, `__destruct`, `__toString`, `__invoke` — find the classes
  from stack traces first (the error names the class).
- `phar://` wrapper: if upload exists, a `phar:` path in any file-read param = object trigger.
- Laravel: `app/` error pages leak class names; `Illuminate\...` = Laravel chains.

## Python
- `pickle.loads` in traces, or ML/pipeline/import endpoints.
- Confirm: send a crafted pickle whose `__reduce__` calls `os.system('echo MARKER')`
  (inert) — echo or timing confirms.
- `marshal` / `shelve` / `yaml.load` (unsafe loader) endpoints: same technique.
- Flask `session` cookie (itsdangerory) — unsigned? `FLASK_SECRET_KEY` from SSTI config dump.

## .NET
- ViewState: `viewstategenerator` — if `MachineKey` recoverable (config leak, SSTI env dump),
  forge with known tools; else note as coverage-gap.
- `LosFormatter` / `BinaryFormatter` in error text = live deserialization.

## Doctrine
- One blind confirm (URLDNS / timing) = deserialization **reachable** — report as
  high-severity with the dependency list as evidence, and open a campaign branch for
  the specific chain once the classpath is confirmed.
- Never fire a full RCE gadget chain at production-looking targets without the task
  constraints allowing it; check the [operational constraints] block first.
