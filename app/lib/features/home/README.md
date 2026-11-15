# features/home

The main tab shell and the Home tab.

## Screens

| Widget | Route | Notes |
|---|---|---|
| `HomeShell` | shell of `/home`, `/events`, `/search`, `/notifications`, `/me` | bottom `NavigationBar` (Home, Events, Search, Alerts, Me); tapping the active tab returns to its first screen; Alerts tab badge = unread notifications (`unreadNotificationsProvider`). Also keeps the unread chat count (`unreadChatProvider`) fresh for the Home chat icon: new / deleted message frames (1 s debounce), app resume and tab change. `CountBadge` is the shared 99+ badge |
| `HomeScreen` | `/home` | the app's feed: app bar with greeting and chat icon (unread badge, opens `/chat`); Discover / Following tabs, each a `FeedList` (features/feed) whose header holds the search entry (switches to the Search tab), quick links (registrations, communities, consultations, premium) and up to 3 featured events (+ "See all events"); new-post button |

## Endpoints

- `GET /events/featured` (through `featuredEventsProvider` in features/events)
- `GET /feed?mode=discover|following&cursor` (through `FeedList` in features/feed)
- `GET /ads?placement=home_feed` (through `AdSlot`)
- `GET /chat/unread-count` (through `unreadChatProvider` in features/chat)
- `GET /notifications/unread-count` (through `unreadNotificationsProvider` in features/notifications)

## Changing it safely

The order of `NavigationBar` destinations must match the order of the `StatefulShellBranch`
list in core/router/app_router.dart. Add a tab in both places together.
