---
name: c2-handoff
description: "Operator-controlled C2 handoff metadata for an authorized task"
---

# C2 handoff

Derived from the KSIR-ARTEX C2 handoff playbook. Afterglow records a review request; the application does not transmit, install, or execute an implant.

Use this playbook only when the operator has enabled `ARTEX_ENABLE_C2_HANDOFF=true` and added the exact target host to this task's manual scope. Keep the binary path in `ARTEX_C2_BIN_X64` or `ARTEX_C2_BIN_X86` under operator control. Never place the binary, its encoded form, or credentials in a task prompt or tool argument.

1. Call `c2_stage_implant` to verify the configured local binary's architecture and hash. The tool does not copy or send bytes.
2. If the operator explicitly requested a handoff, call `c2_deliver_implant` with a declared host, safe absolute target path, and vector. This creates a task record with the binary hash and delivery metadata. The name is retained for compatibility; it does not deliver anything.
3. Wait for the operator to review and acknowledge or cancel the record in the task view. `c2_await_operator` reads that status. Acknowledgement records the review only; it does not prove deployment or check-in.
4. Record any later outcome as evidence from the authorized environment. Do not claim a session, persistence, EDR bypass, or cleanup occurred unless there is independent evidence.

The source's command sequences for binary delivery, persistence, EDR bypass, and log deletion are intentionally absent. Keep audit logs and collected evidence intact unless the operator's engagement procedure directs otherwise.
