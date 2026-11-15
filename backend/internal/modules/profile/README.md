# profile
Public sports profiles: onboarding profile setup, profile view with visibility, follow and DM
rules, profile edits, season stats against sport templates, achievements with certificates, and
"who viewed my profile" for Premium. It also owns the follower counters used by `follow`.

## Routes
See docs/api.md "Profiles, stats, achievements" and "Me, onboarding, verification".

| Method | Path | Group | Permission |
|---|---|---|---|
| POST | `/onboarding/profile` | User | onboarding step must be `profile` |
| GET | `/profiles/:userId` | Verified | target active, onboarded, not blocked either way |
| GET | `/profiles/:userId/stats` | Verified | as above, plus `visibility` and `statsVisibility` |
| GET | `/profiles/:userId/achievements` | Verified | as above, plus `visibility` |
| PUT | `/me/profile` | Verified | own profile |
| GET | `/me/profile-viewers` | Verified | benefit `see_profile_viewers` (403 `PLAN_REQUIRED`) |
| POST | `/me/stats` | Verified | own |
| PUT / DELETE | `/me/stats/:id` | Verified | `WHERE id = ? AND user_id = ?` (another user's id → 404) |
| POST | `/me/achievements` | Verified | own |
| PUT / DELETE | `/me/achievements/:id` | Verified | `WHERE id = ? AND user_id = ?` (another user's id → 404) |

## Tables
- Owned: `profiles`, `profile_sports`, `user_stats`, `achievements`, `profile_views`.
- Writes through helpers: `users.display_name` (`user.SetDisplayName`), `users.onboarding_step`
  (`user.Advance`), `media_uploads` status (via `d.Media` Claim / Purge).
- Read: `users`, `user_roles`, `follows`, `blocks` (via `d.Users`), `user_settings`, `sports`,
  `sport_positions`, `stat_templates`, `identity_verifications` (`verification.BirthYear`),
  `coach_consultations`, `consultation_requests`, `organiser_profiles`, entitlements.
- Exported for `follow`: `AdjustFollowers`, `AdjustFollowing`, `FollowersCount`,
  `HideMinorCities`.

## Events
- Published: none.
- Consumed: `user.deleted`: purges the profile photo and certificate files, clears the profile
  row's free text and location, sets it private, and deletes sports, stats, achievements and
  profile views (both directions). Idempotent; a malformed payload is dropped; DB errors are
  returned for retry.

## Jobs
None.

## Config keys
`minor.profile_default_visibility`, `minor.dm_policy`, `chat.dm_requires_follow`,
`profile.viewers_window_days`, `profile.max_achievements`.

## Rules
- Setup: display name 2-80 characters, an active sport and (optional) a position of that sport,
  text limits (bio 500, city / district / state 64, academy 120); the step must be `profile`
  (409 otherwise). Birth year comes from the verified DOB. Minors get
  `minor.profile_default_visibility` for both visibilities; adults get `public`. The photo must be
  the caller's own `profile_photo` upload (422 otherwise). Onboarding then advances to `done`.
- View: deleted, suspended or unfinished users, missing profiles and blocks either way are all
  404 with the same body. `followers` visibility needs the viewer to follow the target; otherwise
  only the card, counts and relation flags are returned with `restricted: true`. `birthYear` is
  shown only to the owner. A minor's city is hidden unless the viewer follows them.
- A view by an adult non-owner is recorded once per viewer per IST day; minors' views are not
  recorded. A failed record is logged (ids only) and does not fail the request.
- `canMessage` follows docs/api.md "Chat": the target's `allowDmsFrom`, with
  `chat.dm_requires_follow` turning `everyone` into `followers`; when either side is a minor,
  `minor.dm_policy` (`mutual_follow` default and for unknown values, `anyone`, `none`); for adults
  an accepted consultation also allows DMs.
- Update: partial; an empty string clears optional text; `0` clears the photo. A minor may not
  choose `public` unless the minor default is `public`. `coachingSinceYear` is coaches only,
  1900..this year. `sports` lists each sport once, one primary (first by default), all active.
  A replaced photo is purged after commit.
- Viewers: the window is `profile.viewers_window_days` (default 30) days including today, one row
  per viewer (latest day), excluding blocked and inactive users, keyset-paginated.
- Stats: the template must be an active template of the sport; values are checked by field type
  (`int`, `decimal`, `duration_ms`, `percent` 0-100, `text` ≤ 100); unknown keys → 422; one entry
  per sport and season (409). Totals of numeric fields are returned only when the owner has
  `advanced_stats`. Hidden when either visibility hides it.
- Achievements: at most `profile.max_achievements` (default 50, 409 `LIMIT_REACHED`); title 1-120,
  description 500, level from the fixed list, `achievedOn` not in the future (IST). The
  certificate must be the caller's own `achievement_certificate` upload; a replaced or deleted
  certificate is purged. Certificate links are short-lived signed URLs from `d.Media`.
- All SQL is parameterised; the dynamic `SET` list of the update goes through a fixed column map.

## Tests
`profile_test.go` (helpers in `helpers_test.go`): setup happy path (SQL order, birth year, photo
claim, advance), minor default visibility from config, validation, inactive sport / position,
wrong step, photo not owned or wrong purpose; view of a coach and an organiser, own view, every
404 case on all three read routes, followers-only restriction, minor target city and minor viewer,
view-record failure; a `canMessage` table over every config rule; update happy path with photo
purge, clearing and unchanged photo, validation, the minor visibility rule, inactive sport and
missing profile; viewers plan gate, window, cursor and bad cursor; stats create / validation /
duplicate / inactive template, totals with and without the benefit, hidden stats, update and delete
including another user's id; achievements create, limit, defaults, foreign certificate,
validation, partial update with purge, delete, lists; idempotent `user.deleted`; the counter
helpers and `HideMinorCities`.
