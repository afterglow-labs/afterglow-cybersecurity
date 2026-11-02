---
name: osint-identity
description: "Authorized identity exposure and MFA posture assessment with redacted evidence"
---

# Identity exposure assessment

Adapted from the KSIR-ARTEX identity playbook. Work only with identities, tenants, and data sources the operator explicitly authorized. Never acquire credentials or session cookies from criminal markets, perform credential stuffing, send a device-code lure, or run an adversary-in-the-middle login proxy as an automatic agent action.

1. Record the authorized tenant, account set, and assessment method. Use operator-supplied test accounts and keep their passwords in Afterglow's encrypted task account store.
2. Review approved breach or exposure reports by metadata: affected domain, date, source, credential type, and remediation state. Redact passwords, cookies, recovery codes, and access tokens before adding evidence to the task.
3. Inspect identity policy with a tenant-admin session managed by the operator. Check MFA enrollment, phishing-resistant methods, conditional access, stale accounts, and device-code policy. `check_mfa_methods` and `device_code_init` provide scoped assessment plans; they do not authenticate as a user or return tokens.
4. Where an authorized test flow is required, use designated test accounts. Record request IDs, policy decisions, and status codes; do not capture or replay production users' sessions.
5. Report the exact policy gap, affected test identity, and evidence. Recommend disabling unneeded device-code grants, enforcing phishing-resistant MFA where appropriate, monitoring anomalous grants, and rotating any exposed credentials.

The source playbook included instructions for phishing, credential stuffing, token capture, and mailbox access. They are not part of this Afterglow playbook because the assessment tools in this project do not perform those actions and their outputs must not imply that they did.
