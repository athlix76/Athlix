# features/ads

Ad campaigns: create, edit, pause / activate / end, daily stats.

## Screens

| Screen | Route | What it shows |
|---|---|---|
| `AdsScreen` | `/ads` | campaigns, editor, status menu, stats dialog |

## Endpoints

| Method | Path | Permission |
|---|---|---|
| GET | `/admin/ads` | ads.write |
| POST | `/admin/ads` | ads.write |
| PUT | `/admin/ads/:id` | ads.write |
| GET | `/admin/ads/:id/stats` | ads.write |

## Permissions

`ads.write`.

## Folders

- `data/`: API client and models.
- `application/`: providers and pure helpers.
- `presentation/`: screens and dialogs.

## Changing it safely

- Image: "Choose image…" in the editor uploads through `AdsApi.uploadImage` (`POST /admin/media/uploads` purpose `post_image` → presigned PUT → `POST /admin/media/uploads/:id/confirm`, all `ads.write`) and puts the upload id in `imageUploadId`. The S3 key is built by the backend. File picking is `core/media/picked_file.dart`.
- Target URLs must be https.
- If an endpoint or field changes in docs/api.md, change `data/` first, then the screen.
- Writes always use a confirmation dialog and show API error codes; do not bypass it.
