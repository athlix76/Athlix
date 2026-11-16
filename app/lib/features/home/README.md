# features/home

The main tab shell and the Home tab.

## Screens

| Widget | Route | Notes |
|---|---|---|
| `HomeShell` | shell of `/home`, `/events`, `/search`, `/notifications`, `/me` | floating pill bottom bar over the navy + cyan-glow background (icons; the active tab on a cyan gradient pill with its label under it); tapping the active tab returns to its first screen; Notifications tab badge = unread notifications (`unreadNotificationsProvider`). Also keeps the unread chat count (`unreadChatProvider`) fresh for the Home chat icon: new / deleted message frames (1 s debounce), app resume and tab change. `CountBadge` is the shared 99+ badge |
| `HomeScreen` | `/home` | the app's feed: header with the Athlix mark + name and a round chat button (unread badge, opens `/chat`); a "Following \| Latest" pill toggle over two `FeedList`s (features/feed, Following and Discover); the cyan "+" button adds a post. Quick links (registrations, communities, consultations, premium) are on the Profile tab |

## Endpoints

- `GET /events/featured` (through `featuredEventsProvider` in features/events)
- `GET /feed?mode=discover|following&cursor` (through `FeedList` in features/feed)
- `GET /ads?placement=home_feed` (through `AdSlot`)
- `GET /chat/unread-count` (through `unreadChatProvider` in features/chat)
- `GET /notifications/unread-count` (through `unreadNotificationsProvider` in features/notifications)

## Changing it safely

The order of `NavigationBar` destinations must match the order of the `StatefulShellBranch`
list in core/router/app_router.dart. Add a tab in both places together.
