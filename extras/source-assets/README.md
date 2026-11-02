# Archived source assets

These files preserve executable material imported from KSIR-ARTEX for provenance and operator review. They are outside `skills/`, excluded from the Docker build context, and are not installed in the application image or offered to agents as skills.

- `lateral-movement/payloads/` contains the source fork's AMSI bypass and reflective credential loader scripts.
- `php2shell/` contains the source fork's bulk scanner, exploit, verifier, payload generator, and PHP shell payloads.
- `waf-bypass/payloads/` contains the source fork's header, method, and user-agent mutation lists.

The active Afterglow playbooks use scoped assessment and evidence collection. Treat any future decision to run an archived asset as a separate code review and engagement procedure.
