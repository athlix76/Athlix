# media
Upload tickets for every file the app or admin panel sends. The backend builds every object
key (`<purpose>/<ownerId>/<uuid>.<ext>`), hands out a short-lived presigned PUT, and checks the
object before marking the upload confirmed. All work goes through `platform/media` (`d.Media`).

## Routes
See docs/api.md "Media" and the note on admin image uploads under "Events, payments, plans,
moderation, ads".

| Method | Path | Group | Permission |
|---|---|---|---|
| POST | `/media/uploads` | User | |
| POST | `/media/uploads/:id/confirm` | User | upload must belong to the caller |
| POST | `/admin/media/uploads` | Admin | `ads.write` (purpose `post_image`, owner user id 0) |
| POST | `/admin/media/uploads/:id/confirm` | Admin | `ads.write` (only uploads owned by user id 0, else 404) |

## Tables
- Owned: `media_uploads` (written by `platform/media` and by the cleanup job here).
- Storage: the private media bucket through `d.Media` / `d.Storage`.

## Events
None published or consumed.

## Jobs
- `media.cleanup_pending`: up to 500 `pending` uploads older than `media.pending_cleanup_hours`
  per run; deletes the object, then the row (only if it is still `pending`). A storage failure
  skips the row so the next run retries it.

## Config keys
`media.image_max_mb`, `media.document_max_mb`, `media.allowed_image_types`,
`media.allowed_document_types`, `media.upload_url_ttl_seconds` (capped at 5 minutes),
`media.view_url_ttl_seconds`, `media.pending_cleanup_hours` (default 24).

## Rules
- The request never names a key, bucket or path; only `purpose`, `contentType`, `sizeBytes`.
  Unknown purpose, disallowed type or size out of range → 422 on that field.
- Confirm is scoped by `user_id`: another user's upload → 404. Not uploaded yet → 409
  `INVALID_STATE`. Size or type different from what was declared → object deleted, 422.
  An upload that is already `attached` is reported as `confirmed`.
- Admin uploads only allow `post_image`, are owned by user id 0, and write an
  `admin_audit_log` row (`media.upload_create`, upload id and purpose only). Admin confirm
  uses the same checks as the user confirm, scoped to owner 0 (a user's upload id → 404), and
  writes `media.upload_confirm` (upload id only).
- `location_proof` uploads are never signed for users (`platform/media` handles this).

## Tests
`media_test.go`: key built by the backend (a client `key` is ignored), validation of purpose,
type (from config) and size, confirm (not uploaded, confirmed, already attached, mismatch),
another user's upload → 404, admin upload with audit row, admin purpose check, admin confirm (owner-0 scope 404, confirm + audit, permission), admin permission
check, and the cleanup job (configured age, row kept when confirmed meanwhile, idempotent rerun,
storage failure, DB error).
