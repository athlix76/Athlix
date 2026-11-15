# core/media

Browser file picking for admin uploads (ad images).

| File | What |
|---|---|
| `picked_file.dart` | `PickedFile` and `pickFile(accept:)`, the only import other code uses |
| `file_picker_web.dart` | Web implementation (hidden `<input type="file">` through `package:web`) |
| `file_picker_stub.dart` | Non-web builds (unit tests): returns null |

The upload itself is in `features/ads/data/ads_api.dart` (`uploadImage`):
`POST /admin/media/uploads` → presigned PUT to S3 with the returned headers →
`POST /admin/media/uploads/:id/confirm`. The backend builds the object key;
the panel never chooses a path.
