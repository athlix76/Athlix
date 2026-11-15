# core/media

File picking and uploads.

- `pick_image.dart`: `pickImageFile`, `pickImageFiles` (image_picker).
- `pick_file.dart`: `pickDocumentFile(extensions:)` (file_picker; for PDFs such as location proofs).
- `media_upload.dart`: `MediaPurpose`, `UploadFile`, `UploadTicket`, `MediaUploader.upload(purpose, file)`:
  `POST /media/uploads` → PUT bytes to the returned `uploadUrl` → `POST /media/uploads/:id/confirm`.
  Returns the upload id that the feature then sends to its own endpoint.

## Changing it safely

The client never chooses or sees S3 keys: it sends a purpose, content type and size only. Check size and
type against `media.*` config before uploading. Never log the upload URL.
