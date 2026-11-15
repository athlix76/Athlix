# features/profile

Profiles, stats and achievements, and the Me tab.

## Screens

| Screen | Route | Notes |
|---|---|---|
| `ProfileScreen(userId:)` | `/profile/:id` | header, follow, message (`POST /chat/direct`), consultation link, report, block; private profiles show only the header |
| `StatsScreen(userId:)` | `/profile/:id/stats` | entries labelled from the sports config templates; owner can add / edit / delete; career totals when the server sends them |
| `AchievementsScreen(userId:)` | `/profile/:id/achievements` | list with certificate links; owner can add (up to `profile.max_achievements`), edit, delete |
| `EditProfileScreen` | `/me/edit` | name, photo, bio, sports and positions, location, club, coaching year, visibility |
| `ProfileViewersScreen` | `/me/viewers` | needs `see_profile_viewers`; 403 `PLAN_REQUIRED` shows the plan-required view |
| `MyProfileScreen` | `/me` (tab) | own profile, stat templates for my sports, achievements, viewers, links to settings, my events, registrations, organiser verification |

## Endpoints

- `GET /profiles/:id`, `PUT /me/profile`, `GET /me/profile-viewers`
- `GET /profiles/:id/stats`, `POST /me/stats`, `PUT /me/stats/:id`, `DELETE /me/stats/:id`
- `GET /profiles/:id/achievements`, `POST /me/achievements`, `PUT /me/achievements/:id`,
  `DELETE /me/achievements/:id`
- Uploads through core/media (`profile_photo`, `achievement_certificate`)

## Rules

- Stats are **template driven**: inputs, labels and units come from `StatTemplate.fields` in the
  sports config. No stat key appears in code; behaviour depends only on the field `type`.
- Personal values (birth year, location) are shown only as the server returns them; nothing is
  logged. `ProfileUpdate.toString()` prints field names only.
- Minors cannot pick "public" visibility unless `minor.profile_default_visibility` is public.

## Changing it safely

Add a profile field: model `Profile.fromJson` → `ProfileUpdate` → `EditProfileScreen` →
`ProfileBody`. A new stat field type needs a case in `stat_format.dart` (label, parse, format)
and in the keyboard switch of `stat_entry_sheet.dart`.
