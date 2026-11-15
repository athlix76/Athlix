# community

User-created groups. A community is a row in the shared `conversations` table with
`type = 'community'`, and its members are `conversation_members` rows. This module creates and
manages communities and their membership. The chat module owns messages, reads and leaving.
This module does not import chat; it works directly on the shared tables.

## Routes

See [docs/api.md](../../../../docs/api.md) sections "Communities (module community)" and
"Admin panel".

| Method | Path | Group | Who |
|---|---|---|---|
| POST | `/communities` | verified | anyone (limit applies) |
| GET | `/communities?q&sportId&state&cursor&limit` | verified | public, active only |
| GET | `/me/communities` | verified | caller's active / invited / pending |
| GET | `/communities/:id` | verified | public, or private to members / invited / pending |
| PUT | `/communities/:id` | verified | owner or admin (partial) |
| DELETE | `/communities/:id` | verified | owner (sets `archived`, 204) |
| POST | `/communities/:id/join` | verified | per join policy |
| GET | `/communities/:id/join-requests` | verified | owner or admin |
| POST | `/communities/:id/join-requests/:userId/approve` | verified | owner or admin |
| POST | `/communities/:id/join-requests/:userId/reject` | verified | owner or admin |
| POST | `/communities/:id/invites` `{userIds}` | verified | owner or admin |
| POST | `/communities/:id/invites/accept` | verified | invited user |
| POST | `/communities/:id/members/:userId/remove` | verified | owner or admin |
| POST | `/communities/:id/members/:userId/ban` | verified | owner or admin |
| PUT | `/communities/:id/members/:userId/role` `{role}` | verified | owner |
| POST | `/communities/:id/transfer-ownership` `{userId}` | verified | owner; target must be an active admin under their own community limit |
| GET | `/admin/communities?q&status` | admin | `communities.moderate` |
| POST | `/admin/communities/:id/remove` `{reason}` | admin | `communities.moderate` |

## Tables

- Written: `conversations` (rows with `type = 'community'`: title, description,
  avatar_upload_id, owner_id, sport_id, city, state, visibility, join_policy, member_count,
  status) and `conversation_members` (role, status, joined_at).
- Read: `sports` (validate `sportId`), `blocks` (visibility and invites), `follows` (minor
  invites), `users` / `profiles` (through `platform/userdir`), `subscriptions` / plan benefits
  (through `platform/entitlements`), `media_uploads` (through `platform/media`).
- `admin_audit_log` through `audit.Admin` for the admin remove.
- Owned: `community_ownership_transfers` (one row per ownership transfer).
- `conversations.removal_reason` is set by the admin remove.

## Events

Published after commit (`events.CommunityJoinPayload{conversationId, userId, ownerId}`):

- `community.join_requested`: a join to an `approval` community went to pending. Actor = the requester.
- `community.join_approved`: an owner or admin approved a pending request. Actor = the approver.

A publish failure is logged with ids only; the write stands. Nothing is consumed.

## Jobs

None.

## Config keys

| Key | Default | Use |
|---|---|---|
| `chat.max_communities_created_per_user` | `10` | communities one user can own; the limit is max(this, `extra_communities` benefit) |
| `chat.group_max_members` | `500` | active member cap, checked under the row lock on join / accept / approve |
| `minor.can_join_public_communities` | `true` | off → minors get 403 joining or accepting into a public community, and are skipped by invites to one |
| `minor.dm_policy` | `mutual_follow` | invites where either side is a minor: `anyone` passes, `mutual_follow` needs a mutual follow, anything else skips |

Benefit: `extra_communities` (`entitlements.ExtraCommunities`).
Requirement: `create_community` gate (`requirements.GateCreateCommunity`) → 403 `VERIFICATION_REQUIRED` for unverified users when required.

## Rules

- Create: title 3..80 runes after trim (required), description ≤ 500, city / state ≤ 64,
  `visibility` public | private (default public), `joinPolicy` open | approval | invite (default
  open, or invite for private). Private + open → 422. Unknown or inactive `sportId` → 422. The
  avatar must be the caller's `community_avatar` upload, claimed with `d.Media.Claim` inside the
  tx; otherwise 422. The conversation and the owner member row are written in one tx.
- Owned limit: only `status = 'active'` communities count, so archiving one frees a slot. A
  per-user Redis lock (`community:create:<userId>`, 10 s) stops parallel creates passing the
  limit together (409 `CONFLICT` while held). Over the limit → 409 `LIMIT_REACHED`.
- Visibility: archived / removed communities are 404 to everyone in the app. A private
  community is 404 to anyone without an active, invited or pending row. A block between the
  viewer and the owner (either direction) → 404. Visible but not allowed → 403.
- `myStatus` is `active | pending | invited | banned | none`; `myRole` is set only when active.
- Join: open → active; approval → pending + `community.join_requested`; invite → 403 unless
  invited (an invited user's join accepts the invite). Banned → 403. Active or pending →
  idempotent 200 with the current status. Accept without an invite → 404.
- Member changes lock the `conversations` row (`FOR UPDATE OF c`), so `member_count` and the
  cap check are consistent. `member_count` moves only on active ↔ non-active transitions.
- Invites: 1..50 ids, deduplicated, response `{invited, skipped}`. Skipped: self, unknown or
  inactive / not onboarded users, existing active / pending / banned members, users blocked
  either way with the inviter or the owner, and minors who fail the minor rules above. Already
  invited users are reported as invited and not rewritten.
- Remove / ban / role: nobody acts on themselves here (409 `INVALID_STATE`, use leave). Nobody
  acts on the owner; admins cannot act on other admins (403). Only the owner changes roles
  (`admin` | `member`, target must be active). Ban works on non-members too (pre-emptive).
- PUT is partial: a missing key is unchanged, `null` clears a nullable field.
- Admin remove: reason required (≤ 500), sets `removed`, writes `audit.Admin` in the same tx
  with `previousStatus` and `reasonLength` only (no free text in audit details). Removing an
  already removed community is a 204 no-op without a second audit row.
- Avatar URLs come from `d.Media.ViewURLs`, only for rows that passed the visibility check.
  Logs carry ids only. All SQL uses bind parameters; `LIKE` wildcards in `q` are escaped.

## Tests

`community_test.go` (sqlmock + testutil):

- create happy path; limit from the setting; limit raised by the entitlement; 422 validation;
  unknown sport and foreign avatar; verification requirement
- private community 404 to an outsider; banned member's view; browse filters and cursor; empty mine
- partial update; private + open rejected; plain member 403; delete owner-only and archive
- join per policy (open, approval with event after commit, invite-only, invited, banned, private
  outsider, minor blocked / allowed); idempotent join; `chat.group_max_members` reached; accept invite
- approve publishes `community.join_approved` after commit; approve rules; reject and list requests
- invites with skipped (self, active member, minor without mutual follow); minor policy anyone / none;
  invite validation and permission
- role change owner-only; remove / ban rules and count changes
- admin list, 403 without `communities.moderate`, audited remove, missing reason, unknown id

```
cd backend && go test -count=1 ./internal/modules/community/
```
