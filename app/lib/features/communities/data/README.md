# features/communities/data

Wire models and REST calls for communities.

## Contents

- `community_models.dart`:
  - `Community` and `CommunityInput` (`toJson(only:)` builds partial `PUT` bodies)
  - `JoinRequest` and `InviteResult`
  - wire constants: `CommunityVisibility`, `JoinPolicy`, `MembershipStatus`
- `community_api.dart`: `CommunityApi` and `communityApiProvider`. It includes `searchPeople`,
  which reads each `PersonResult` as a `UserSummary`.

## Changing it safely

Field names match docs/api.md exactly. For a new input field, add it to `CommunityInput.toJson`
and to the changed-field check in `community_form_screen.dart`. Leaving a community and listing
its members use `chat/data/chat_api.dart`, not this file.
