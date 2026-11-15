# features/communities/application

Riverpod state for communities.

## Contents

- `community_providers.dart`:
  - `communityProvider(id)`
  - `communityFiltersProvider`: the Discover filters, kept while the app runs
  - `CommunityController`: every write, including the avatar upload as `community_avatar`. After
    a write it invalidates both `communityProvider(id)` and the chat `conversationProvider(id)`,
    because they share the id.

## Changing it safely

Put new writes in `CommunityController`, so screens never call the API directly for a change.
Invalidate both providers after any write that changes membership or details.
