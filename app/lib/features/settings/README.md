# features/settings

Settings, account and legal links.

## Screens

| Screen | Route | Notes |
|---|---|---|
| `SettingsScreen` | `/settings` | push categories, privacy, location identity (if `location.enabled`), blocked users, add a role, organiser / consultation links, withdraw guardian consent (minors with granted consent), policies, support, log out, delete account, app version |

## Endpoints

- `GET /me/settings`, `PUT /me/settings` (partial: one whole group per call)
- `POST /me/roles` `{role}` → Me
- `POST /me/guardian/withdraw/request` + `/verify` (through `GuardianApi`): the code goes to the
  guardian's phone; on success `setMe` and the router sends the user back to the guardian step
- Via other code: `DELETE /notifications/devices` (push stop), `POST /auth/logout`, `DELETE /me`
  (session controller)

## Rules

- Log out and delete account stop push **first** (the device DELETE needs the session).
- Delete account always asks for confirmation.
- Policies link to every `PolicyDocType` value; add new ones to `_policyDocTypes`.

## Changing it safely

New setting: add it to `data/settings_models.dart` (`fromJson`, `toJson`, `copyWith`), a setter
on `SettingsController`, then a tile. Send the whole group so the server never sees half a group.
