# backend/scripts/smoke

End-to-end smoke test for a **local** API. It drives the real HTTP endpoints the app and admin
panel use and checks the main business rules in about a minute. It refuses any base URL that is
not `127.0.0.1` / `localhost`, and it never prints tokens, passwords or OTPs.

## What it checks (23 checks)

- OTP login (fixed dev OTP), onboarding, mock DigiLocker verification, policy acceptance
  (only general policies block; guardian consent and the location notice do not)
- DigiLocker district becomes the native location identity
- Admin login with password + TOTP; a wrong password is rejected
- Organiser verification submitted and approved by the admin
- Listing fee from `listing_fee_rules`: 5% x Rs 500 x 16 slots + 18% GST = Rs 472.00; the admin
  changes the rule to 10% and the next quote is Rs 944.00 with no restart; free events use the
  free-event fee
- Config write with optimistic locking (stale `version` gets 409), and the
  `event.require_admin_approval` switch changing what happens after payment
- Event with a "Thanjavur natives only" rule: a Coimbatore native does not see it in the feed,
  sees it greyed with `showAll=true`, gets 403 `NOT_ELIGIBLE` on register and cannot read the
  organiser contact; a Thanjavur native sees it, registers (`qualifiedVia: native`) and gets the
  contact; the organiser sees the `qualifiedLabel`
- Ownership: another user editing the event or listing its registrations gets 404; no token gets 401

## Core flows test (`core_flows_test.py`, 28 checks)

Covers what `smoke_test.py` does not, with two fresh users and no admin account: player and
coach onboarding, profile with sports, people and global search, follow (+ notification), posts
in the Following and Discover feeds, like and comment (+ notifications), ownership on posts,
direct message and unread count, notifications list and mark-all-read, 401 without a token,
and account deletion (token stops working, the user's messages and posts are hidden).

```bash
python3 -I scripts/smoke/core_flows_test.py http://127.0.0.1:8080/v1
```

It ends with `ALL 28 CHECKS PASSED`. Same local setup as below.

## Extended flows test (`extended_flows_test.py`, 57 checks)

Phase 4 coverage of everything the other scripts skip: auth edge cases (wrong OTP, resend
cooldown 429, refresh rotation, reuse of an old refresh token revokes the whole family, logout,
tampered tokens), minors (guardian OTP consent, too-young refused, no stranger DMs until a mutual
follow, followers-only profile, no ads), stats from the sport template (unknown key 422) and
achievements, private profiles, privacy settings (hidden from search, DMs off), block, photo post,
communities (open / approval / ban / admin removal), organiser tools (1-slot event: waitlist,
event group chat, mark paid, withdraw promotes the waitlisted player), coach consultations
(offer, request, accept, complete), premium mock purchase and ads (both switched on for the test
and restored), and error handling (422 envelopes, malformed JSON 400, unknown route 404, >1 MB body).
Same admin env vars as `smoke_test.py`; about 3 minutes (it waits out OTP cooldowns and the 15 s
settings cache).

## Data integrity check (`integrity_check.sql`)

The schema has no foreign keys, so this query looks for orphaned rows (registrations, posts,
comments, likes, messages, memberships, follows, orders, notifications), leftovers of deleted
users (device tokens, visible messages) and counter drift (`registrations_count`,
`followers_count`). Every row must print 0:

```bash
docker compose exec -T mysql mysql -uathlix -pathlix-local athlix -N < backend/scripts/smoke/integrity_check.sql
```

## Admin flows test (`admin_flows_test.py`, 38 checks)

Everything the admin panel does, through its API: dashboard; user search (masked mobile),
detail, suspend / unsuspend; a player's residence proof uploaded, queued, opened (signed URL)
and approved; organiser verification rejected with a note, resubmitted and approved; tournament
review (pending -> approve -> feature / unfeature -> remove; reject with the configured listing
refund); listing orders and fee preview; reports resolved with `hide` (post disappears) and
`dismiss`; config change + wrong type (422) + history; a limited role (`users.read` only) gets
403 on config, suspend and reports; a disabled admin is cut off; user tokens cannot call admin
APIs; the audit log records the actions. Needs the same admin env vars as `smoke_test.py`
(a super_admin); it waits for fresh TOTP windows, so it takes about 1.5 minutes.

```bash
SMOKE_ADMIN_EMAIL=... SMOKE_ADMIN_PASSWORD='...' SMOKE_ADMIN_TOTP_URI='otpauth://...' \
  python3 -I scripts/smoke/admin_flows_test.py http://127.0.0.1:8080/v1
```

It creates users, events, a role and a (then disabled) admin in the local DB.

## Run it

Needs Python 3 (standard library only), a migrated local DB and the API running with
`ENV=local`, `DEV_FIXED_OTP=123456`, `verification.provider = mock` and
`listing.payment_provider = mock` (the seed defaults).

```bash
cd backend
make dev-secrets migrate
make create-admin EMAIL=smoke@athlix.local NAME="Smoke Admin"   # prints a temporary password + TOTP URI once
make run                                                         # in another terminal

SMOKE_ADMIN_EMAIL=smoke@athlix.local \
SMOKE_ADMIN_PASSWORD='<temporary password>' \
SMOKE_ADMIN_TOTP_URI='<otpauth://... URI>' \
  python3 -I scripts/smoke/smoke_test.py http://127.0.0.1:8080/v1
```

It ends with `ALL 23 CHECKS PASSED`, or prints the first failing call with its status and body.

Notes:
- Each run uses new mobile numbers (OTP cooldown). Admin TOTP codes are single-use per 30 s
  window, so the script waits for a fresh window before logging in.
- It restores `event.require_admin_approval = true` and the fee rule to 5% at the end. If a run
  is aborted halfway, the next run puts the approval switch back first.
- It creates users and an event in your local DB; drop and re-migrate the local DB to reset.

## Changing it safely

Add checks with `check(cond, label, detail)` after the step that needs them. Keep it standard
library only and keep the local-only URL guard.
