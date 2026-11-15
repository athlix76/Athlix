# consultation

Coach consultation listings and player requests. Athlix only lists the offer; the fee is
agreed and paid **directly to the coach** (the free-text `feeNote` is all Athlix stores).

## Routes

See [docs/api.md](../../../../docs/api.md) section "Coach consultations".

| Method | Path | Group | Who |
|---|---|---|---|
| GET | `/coaches/:userId/consultation` | verified | anyone not blocked; 404 if not offered, unless self |
| PUT | `/me/consultation` | verified | coaches (else 403 `FORBIDDEN`) |
| GET | `/me/consultation` | verified | coaches (else 403 `FORBIDDEN`); own offer, live or not |
| POST | `/coaches/:userId/consultation-requests` | verified | any user except the coach |
| GET | `/me/consultation-requests?as=coach\|player&status` | verified | caller's own requests |
| POST | `/consultation-requests/:id/respond` | verified | that coach |
| POST | `/consultation-requests/:id/cancel` | verified | that player |
| POST | `/consultation-requests/:id/complete` | verified | that coach |

## Tables

- Owned: `coach_consultations`, `consultation_requests`.
- Read (through platform): `users` / `user_roles` / `profiles` / `blocks` via `d.Users`,
  `subscriptions` / plan benefits via `d.Entitlements`, `verification_requirements` via `d.Requirements`.

## Events

| Published | When | Payload |
|---|---|---|
| `consultation.requested` | after a request is committed | `ConsultationPayload` (status `pending`) |
| `consultation.responded` | after accept / decline is committed | `ConsultationPayload` (status `accepted` / `declined`) |

None consumed. Cancel and complete publish nothing (no event in docs/architecture.md).

## Jobs

None.

## Config keys

| Key | Default | Use |
|---|---|---|
| `consultation.max_open_requests_per_player` | `5` | pending requests a player may have (409 `LIMIT_REACHED`) |

`verification_requirements` row `coach / coach_pro_subscription / offer_consultation` (seed: off)
decides whether a coach needs the `consultation_listing` benefit.

## Rules

- A coach is visible only if active, has the `coach` role and neither side blocked the other;
  otherwise 404.
- When the requirement is on, offering needs `consultation_listing` (403 `PLAN_REQUIRED`,
  `fields.benefit = "consultation_listing"`), and a listing whose coach lost the benefit is
  hidden from others (404) and cannot take requests.
- `PUT /me/consultation` is partial: a missing key is unchanged, `null` / blank clears
  `description` / `feeNote`. `modes` ⊆ `online, in_person`, stored de-duplicated in that
  order, and must be non-empty while `offered`.
- Requests: not to yourself (422), `message` 1..1000 chars, `preferredMode` must be one the
  coach offers (422). One pending request per coach-player pair (409 `CONFLICT`) and at most
  `consultation.max_open_requests_per_player` pending per player (409 `LIMIT_REACHED`), checked
  under `SELECT ... FOR UPDATE` in the insert transaction.
- State machine: `pending → accepted | declined` (coach), `pending → cancelled` (player),
  `accepted → completed` (coach); anything else 409 `INVALID_STATE`. Every lookup is scoped in
  SQL by `coach_id` / `player_id` from the token, so other users get 404.
- The `note` on respond is accepted but not stored (no column).
- Logs carry ids and status only; messages are never logged.

## Tests

`consultation_test.go`: listing view with `canRequest` (true, pending with coach, at limit),
blocked / not a coach / not offered / plan lapsed → 404, self view without a row, PUT coach-only
403, `PLAN_REQUIRED` with the requirement on and success with the benefit, default (no plan
needed) partial update with mode clean-up, validation 422s, request create with
`consultation.requested` payload, self / validation / unsupported mode 422, duplicate 409,
default limit 409 and a raised limit passing, not offered 404, list validation and paging,
respond scoped to the coach (404), invalid state 409, accept publishing
`consultation.responded`, cancel by the player, complete only from accepted, coach cannot cancel.
