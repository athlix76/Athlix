# lib/core/widgets

Shared widgets: shell and sidebar, permission gate, error view, async body, cursor list, confirm dialog, editor dialog, form fields, date picker.

| File | What it is |
|---|---|
| `admin_shell.dart` | Sidebar layout around every signed-in screen. |
| `api_error_view.dart` | Inline error box: title, code, server message, hint, per-field messages and request id. |
| `async_body.dart` | Renders an [AsyncValue]: spinner, error (403 → forbidden view) or data. |
| `common.dart` | Page header + optional filter bar + body that fills the rest. |
| `confirm_action.dart` | Asks for a reason inside the confirmation dialog. |
| `cursor_list.dart` | A list over a cursor-paginated endpoint with "Load more". |
| `date_time_field.dart` | Picks a local date + time; the value is kept as UTC. |
| `editor_dialog.dart` | A form dialog for create / edit screens. |
| `permission_gate.dart` | Shows [child] only when the admin has one of [anyOf]; else the 403 view. |

## Changing it safely

- Every write goes through `confirmAction` or `showEditorDialog`; they show API error codes and keep the dialog open on failure.
- Every list or detail uses `AsyncBody` or `CursorList`, so a 403 shows the forbidden view.
