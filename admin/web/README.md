# web/

The HTML shell for the Flutter web build.

| File | What it is |
|---|---|
| `index.html` | Page shell. Holds the strict Content-Security-Policy, `no-referrer` and `noindex`. |
| `flutter_bootstrap.js` | Template Flutter fills in at build time (`{{flutter_js}}`, `{{flutter_build_config}}`). Loads CanvasKit from our own origin and registers no service worker. |

## The CSP and why each source is there

| Directive | Value | Why |
|---|---|---|
| `script-src` | `'self' 'wasm-unsafe-eval'` | Our JS plus CanvasKit WebAssembly. No inline scripts, no CDN. |
| `style-src` | `'self' 'unsafe-inline'` | The Flutter engine sets inline styles on its host elements. |
| `connect-src` | `'self'`, the API origin, `https://fonts.gstatic.com`, the media bucket origin | API calls; fallback fonts the engine downloads; proof and ad images (CanvasKit fetches images with XHR, so the signed S3 GET URL is a `connect-src`) and presigned PUT uploads (ad images). |
| `img-src` | `'self' data: blob:`, the media bucket origin | Images from signed S3 URLs. |

The media bucket origin is `https://*.s3.ap-south-1.amazonaws.com` in the source (presigned
URLs are virtual-hosted, regional: `https://<bucket>.s3.ap-south-1.amazonaws.com/...`). The
deploy step narrows it to the exact bucket:

```sh
ORIGIN=$(terraform -chdir=infra/envs/prod output -raw media_bucket_origin)
sed -i "s#https://\*\.s3\.ap-south-1\.amazonaws\.com#${ORIGIN}#g" build/web/index.html
```
| `frame-ancestors` | set as a **response header** only | A `<meta>` CSP cannot set it. CloudFront sends `frame-ancestors 'none'`. |

## Changing it safely

- The API origin in `connect-src` is `http://localhost:8080` for local runs. The deploy
  step replaces it with the real origin (see `../README.md`). If you change the API host,
  change the deploy step and the CloudFront policy too.
- Do not add a CDN, `'unsafe-eval'`, or inline `<script>`. If a package needs one, find
  another package.
- Keep the source value `https://*.s3.ap-south-1.amazonaws.com` (only S3 in our region, never
  all of `*.amazonaws.com`); the deploy step above swaps in the exact bucket origin. If the
  region or the URL style (virtual-hosted) changes, change both. The bucket's CORS rule
  (`infra/modules/s3_cdn`, origins from `admin_origins`) allows `GET`/`HEAD` and `PUT` from the
  admin origins; without it proof images and ad uploads fail.
- After any change, open the panel, sign in, open a location proof and check the browser
  console for CSP violations.
