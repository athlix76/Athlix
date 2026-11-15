# lib/core/router

`go_router` setup and the sidebar list. Every route is wrapped in the same permission gate as its sidebar item.

| File | What it is |
|---|---|
| `app_router.dart` | Wraps a screen in the permission gate of its nav item. |
| `nav.dart` | One sidebar entry. |

## Changing it safely

- Add a screen to `navItems` and `app_router.dart` together; `_screen()` gates it automatically.
- Never put sensitive values (reasons, names, tokens) in the URL. Pass them as route `extra`.
