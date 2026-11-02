---
name: lateral-movement
description: "Scoped lateral movement assessment plan and operator evidence handoff"
---

# Lateral movement assessment

Adapted from the KSIR-ARTEX lateral movement playbook. Afterglow's `lateral_scan`, `harvest_credentials`, `move_lateral`, `escalate_privilege`, `domain_escalate`, `check_edr`, and `cleanup_artifacts` tools create scoped plans or review local evidence. They do not dump credentials, move to another host, disable EDR, erase logs, or clean a target. Never describe a plan as a completed action.

1. Confirm the task's authorized hosts, network range, accounts, methods, and time window. A target outside a manually declared scope must not be used for a handoff.
2. Use `lateral_scan` to enumerate the permitted methods and required evidence for each scoped host. Inspect only operator-provided network results or data collected through the application's guarded tools.
3. Use the encrypted task account store for designated test credentials. `harvest_credentials` describes potential exposure paths but must not return password or token values. Reference a credential by its task-scoped ID when preparing a plan.
4. Use `move_lateral`, `escalate_privilege`, or `domain_escalate` to record an authorized hypothesis, prerequisites, target, and expected evidence. An operator decides whether to run a test through the existing guarded execution tools and records what actually happened.
5. Use `check_edr` to document observed protection status. The tool does not bypass or disable a product.
6. Use `cleanup_artifacts` to inventory test files for operator review. Keep system audit logs and finding evidence intact. Confirm cleanup from independent evidence before recording it as complete.

When reporting, distinguish **proposed**, **attempted**, and **verified** outcomes. Include the exact scoped host and evidence reference. Avoid raw credential values, service tokens, and unverified success claims in model-visible output.
