# Optional scoped gateway API

APTEX runs independently with its full client and server. The gateway API is
available for custom integrations and was originally used by the embedded
Odysseus Cybersecurity workbench.

Send `POST /api/auth/gateway-token` with an administrator JWT in the bearer
header. The response contains a scoped token and its expiration time. It lasts
30 days and can be renewed with another administrator request.

The token is accepted only in a bearer header and only for its fixed API
methods and paths. It cannot read task credentials, change global settings, or
issue another token. It permits task control and interception decisions, so
keep it server-side and protect the APTEX address. Model discovery, tests, and
profile creation accept the fixed local Ollama endpoints or ChatGPT-plan
profile shape; administrator JWTs retain the full setup API.

The standalone APTEX launcher initializes its own administrator when needed
and does not create or install gateway credentials in another application.
