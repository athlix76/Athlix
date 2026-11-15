# admin/

The Athlix admin panel: a Flutter Web app for the operations team. Through it they review
users, organisers, events and location proofs, handle reports and moderation, manage fees,
plans, ads and broadcasts, edit runtime config and policies, and manage admins and roles.

Contract: the "Admin panel" section of [docs/api.md](../docs/api.md) and its conventions.
Security rules: [docs/security.md](../docs/security.md), "Admin panel".

## Layout

| Path | What it is |
|---|---|
| `lib/core/` | API client, error mapping, permissions, token store, router, shared widgets |
| `lib/features/<area>/` | One folder per area, each with `data/`, `application/`, `presentation/` and a README listing its screens, endpoints and permissions |
| `web/` | `index.html` with the strict CSP, and `flutter_bootstrap.js` |
| `test/` | Unit tests: permission gating, API error mapping, config value validation, fee preview, proof link expiry |

## Setup

You need Flutter 3.24 or newer (stable) with web support.

```sh
cd admin
flutter pub get
```

## Run locally

Start the API first (see `backend/README.md`; it listens on `http://localhost:8080` and allows
CORS from `http://localhost:5050`). Then run:

```sh
flutter run -d chrome --web-port 5050 --release
# against another API:
flutter run -d chrome --web-port 5050 --release --dart-define=API_BASE_URL=https://api.dev.example.com/v1
```

**Use `--release` (or `--profile`) locally.** A plain debug `flutter run -d chrome` shows a blank
white page: the debug tooling injects an inline `<script>` (`dwds/.../client.js`), and the strict
CSP in `web/index.html` (`script-src 'self' 'wasm-unsafe-eval'`) blocks it, so the app never
starts. Do not loosen the CSP for this; release mode has no injected script. Release mode has no
hot reload: stop and run again after a change.

**Why 5050, not 5000:** on macOS, port 5000 is taken by AirPlay Receiver (ControlCenter).

| `--dart-define` | Default | Meaning |
|---|---|---|
| `API_BASE_URL` | `http://localhost:8080/v1` | API base including `/v1` |
| `IDLE_TIMEOUT_MINUTES` | `15` | Sign out after this long with no mouse or keyboard input |

The port must be 5050, because the backend's default `ADMIN_ORIGINS` allows that origin (use
another port only together with `ADMIN_ORIGINS=http://localhost:<port> make run`). Create the
first admin with `admincli` (see `backend/README.md`), then sign in with email, password and
the TOTP code.

## Test and analyse

```sh
flutter analyze
flutter test
```

## Session and token rules

- Access and refresh tokens are kept **in memory only**: no localStorage, sessionStorage,
  cookies or IndexedDB. Reloading or closing the tab signs the admin out. This is intended.
- The access token is refreshed shortly before it expires, and once again after a 401. If the
  refresh fails, the session ends and the login screen explains why.
- After `IDLE_TIMEOUT_MINUTES` with no input, the panel calls logout and drops the tokens.
- Nothing logs requests, responses, tokens, TOTP codes or PII. The `avoid_print` lint is on.
- Sensitive reads are confirmed first and audited by the backend: reading reported messages
  (with a reason), revealing a verified name, and viewing a location proof. The message-read
  reason is never put in the browser URL.

## Build

```sh
flutter build web --release --dart-define=API_BASE_URL=https://api.example.com/v1
```

The output is in `build/web/`. CanvasKit is copied into `build/web/canvaskit/`, which the
bootstrap uses so no CDN is needed.

## Deploy: S3 + CloudFront, behind Cloudflare Access or an IP allowlist

1. **Put the real API origin in the CSP.** `web/index.html` lists `http://localhost:8080` in
   `connect-src`. Replace it in the build output, never in the source:
   ```sh
   sed -i 's#http://localhost:8080#https://api.example.com#' build/web/index.html
   ```
2. **Upload to a private S3 bucket** (no public access, no website hosting). Read access is
   only through a CloudFront Origin Access Control.
   ```sh
   aws s3 sync build/web/ s3://<admin-bucket>/ --delete
   ```
   Set `Cache-Control: no-cache` on `index.html`, `flutter_bootstrap.js` and `main.dart.js`,
   so a new release is picked up at once. Hashed assets may be cached.
3. **CloudFront distribution** with the S3 origin, HTTPS only, and the default root object
   `index.html`. Add a response headers policy:
   - `Content-Security-Policy`: the same value as in `web/index.html` (after step 1), plus
     `frame-ancestors 'none'`
   - `Strict-Transport-Security: max-age=63072000; includeSubDomains`
   - `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`,
     `X-Frame-Options: DENY`, `Permissions-Policy: camera=(), microphone=(), geolocation=()`
   - The panel uses Flutter's default hash URLs (`/#/users/42`), so no rewrite rule is needed.
     If you switch to path URLs, map S3 403 and 404 to `/index.html` with status 200.
4. **Restrict who can reach it.** Put the admin hostname behind **Cloudflare Access** (SSO for
   the ops team) or a Cloudflare WAF rule that allows only the office / VPN egress IPs. This
   is in addition to the panel's own login, not instead of it.
5. **Allow the origin on the API.** Add `https://admin.example.com` to `admin_origins` in the
   infra tfvars (`ADMIN_ORIGINS`), which sets CORS on the API and on presigned uploads. Never
   use `*`.
6. **Allow proof images.** The private media bucket's CORS must allow `GET` from the admin
   origin, because the location proof viewer loads signed S3 URLs with XHR. Narrow
   `https://*.amazonaws.com` in the CSP to that bucket's host.

After a deploy, sign in, open one screen from each sidebar section and a location proof, and
check the browser console for CSP or CORS errors.

## Adding a screen

1. Check the endpoint and its permission in docs/api.md. If it is missing, do not invent it:
   raise it with the backend owner.
2. Create `lib/features/<area>/data/<area>_api.dart` (calls through `ApiClient`, models parsed
   with `core/api/json.dart`), `application/<area>_providers.dart`, and
   `presentation/<area>_screen.dart`.
3. Build the screen from the shared widgets: `PageScaffold`, `CursorList` or `AsyncBody` (they
   show the 403 view), and `confirmAction` / `showEditorDialog` for every write (they confirm
   first and show API error codes).
4. Gate write buttons with `ref.watch(sessionProvider).can(Perm.x)`.
5. Add a `NavItem` in `lib/core/router/nav.dart` and a `_screen(...)` route in
   `lib/core/router/app_router.dart` with the same path. The route gets the same permission
   gate as the sidebar item.
6. Write the feature `README.md` (screens, endpoints, permissions) and a README in each new
   folder. Add unit tests for any pure logic.
7. Run `flutter analyze` and `flutter test`.

## Changing it safely

- Never hard-code business numbers (fees, limits, prices, durations). They come from the API
  or config.
- Money is integer paise; percentages are basis points.
- Show masked PII only. Never copy a signed URL, proof, verified name or temporary password
  into state that outlives its screen or dialog.
- Do not add packages that need a CDN, `eval` or inline scripts; the CSP blocks them.
