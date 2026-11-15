# features/communities/presentation

Community screens. They hold UI state only.

## Contents

- `communities_screen.dart`: `CommunitiesScreen` (Discover and Mine tabs)
- `community_tile.dart`: `CommunityTile`, a list row with a status chip
- `community_detail_screen.dart`: `CommunityDetailScreen`, with actions chosen by `myStatus` /
  `myRole`
- `community_form_screen.dart`: `CommunityFormScreen` (create or edit)
- `community_manage_screen.dart`: `CommunityManageScreen` (Members, Requests and Invite tabs)

## Changing it safely

- Use `Routes.*` for navigation.
- Pick the avatar with `pickImageFile` and check it with `MediaUploader.validate` before
  uploading.
- Report a community with `ReportTarget.conversation` (the community id is the conversation id).
- Treat role checks here as UI hints only. The server is the authority.
