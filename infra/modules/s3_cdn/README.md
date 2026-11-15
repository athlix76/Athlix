# modules/s3_cdn

The private media bucket, and an optional CloudFront distribution.

## What it creates

- Bucket with all public access blocked, owner-enforced objects, AES256 default
  encryption, optional versioning (noncurrent versions expire after 30 days), and
  aborted multipart uploads cleaned after 1 day.
- CORS for presigned GET/HEAD (location proof and ad images in the admin panel) and PUT (ad
  image uploads) from the given origins (`admin_origins` via `modules/stack`; headers
  `Content-Type`, `Content-Length`, `x-amz-server-side-encryption`). Never `*`. CORS opens
  nothing by itself: every request still needs a presigned signature.
- Bucket policy denying non-TLS access.
- Optional CloudFront with Origin Access Control, allowed to read only
  `cdn_public_prefixes` (default `event_banner`). Private purposes (profile photos,
  certificates, chat images, organiser documents, location proofs) are rejected by validation.

The backend decides every object key (`<purpose>/<userId>/<uuid>.<ext>`) and hands out
short-lived presigned URLs; this module never grants clients direct access.

## Outputs

`bucket_name`, `bucket_arn`, `bucket_origin` (`https://<bucket>.s3.<region>.amazonaws.com`, the
host of every presigned URL; the admin web deploy writes it into the CSP as
`media_bucket_origin`), `cdn_domain_name`, `cdn_distribution_id`.

## Changing it safely

Renaming the bucket replaces it (data loss unless copied). Keep `force_destroy` false in
prod. Adding a CDN prefix exposes every object under it to anyone with the URL: only
prefixes that are public in the app.
