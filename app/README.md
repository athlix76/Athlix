# Athlix mobile app

Flutter app for Android and iOS. It talks only to the Athlix API (`docs/api.md` is the contract).

## Setup

Needs Flutter 3.24+ / Dart 3.5+.

```
cd app
flutter create --platforms=android,ios --org com.athlix .   # generates android/ and ios/ (keeps lib/, test/)
flutter pub get
```

Then:

1. Delete `test/widget_test.dart` if `flutter create` added it (it references a `MyApp` demo
   class that does not exist and breaks `flutter test`).
2. Apply the platform edits in [android/README.md](android/README.md) (permissions, deep link,
   plugin compileSdk 36, debug-only cleartext) and [ios/README.md](ios/README.md) (usage strings,
   deep link; iOS uses CocoaPods, set in `pubspec.yaml`).

Locally the Android emulator is the easiest way to click through the app (Xcode 27 has no
Simulator window, see ios/README.md).

## Run

```
flutter run --dart-define=API_BASE_URL=http://10.0.2.2:8080/v1 --dart-define=ENV=local
```

| Define | Default | Notes |
|---|---|---|
| `API_BASE_URL` | `http://10.0.2.2:8080/v1` | Includes `/v1`. `10.0.2.2` is the host from the Android emulator; the iOS simulator uses `http://localhost:8080/v1`. |
| `ENV` | `local` | `local`, `dev`, `staging`, `prod`. Mock flows (DigiLocker, listing payment, premium) are hidden in `prod`. |

There are no other build settings and **no secrets in the app**. Every business number (fees, limits,
OTP length, minimum version, radius limits, eligibility defaults) comes from `GET /bootstrap` and is read
through `AppConfig` (`lib/core/config/app_config.dart`). Never hard-code one.

## Deep link

DigiLocker returns to `athlix://verification/result?...`. Flutter drops the host, so the router sees
`/result` and redirects to `/verification/result` with the query kept (`lib/core/router/redirect.dart`).
Platform registration is in the android / ios READMEs.

## Permissions

| Permission | Why | Asked when |
|---|---|---|
| Camera | profile photo, posts, verification and location proof documents | user taps "Take photo" |
| Photos | same, from the gallery | user picks an image |
| Notifications | push (FCM) | after sign-in |
| Location (coarse, when in use) | "Near me" distance filter on events | user taps a radius chip |

Device location is used once to build the query (`lat`, `lng`, `radiusKm`), kept in memory only, and never
stored, logged or sent anywhere else. If permission is denied the radius chips are hidden.

## Firebase

Firebase native files are environment specific and **gitignored**:
`android/app/google-services.json`, `ios/Runner/GoogleService-Info.plist`, `lib/firebase_options.dart`.
Get them from the Firebase console for the right project. The app initialises Firebase lazily when push
starts, so it still runs without them (push is then off).

## Architecture

```
lib/
  main.dart, app.dart     entry point, MaterialApp.router, starts push on sign-in
  core/                   shared plumbing (API client, auth, config, router, session, widgets, ...)
  features/<feature>/
    data/                 API calls and wire models (field names match docs/api.md)
    application/          Riverpod providers / controllers, pure view logic
    presentation/         screens and widgets
test/                     pure unit tests
```

- State: Riverpod 2. Navigation: go_router with one redirect rule (`computeRedirect`). HTTP: dio.
- Responses are `{data, meta?}` or `{error:{code,message,fields}}`; code switches on `error.code` only.
- Tokens live only in `flutter_secure_storage`; refresh is single-flight because refresh tokens rotate.
- Money is integer paise everywhere (`lib/core/money`). Athlix never collects entry fees.
- Uploads: ask `POST /media/uploads` by purpose, PUT to the returned URL, confirm. The app never builds or
  sees S3 keys.
- Minors' safety rules are enforced by the server; the app only renders what it is allowed to see.
- Location eligibility is decided by the server (`myEligibility`, `NOT_ELIGIBLE`); the app never evaluates
  rules itself.
- Never log tokens or PII. Models holding sensitive values redact `toString()`.

## Adding a feature

1. `lib/features/<name>/{data,application,presentation}` with a README in each folder.
2. Models: `fromJson` / `toJson` with the exact field names from docs/api.md, using `core/json` helpers.
3. Route constant in `core/router/routes.dart`, registration in `app_router.dart`.
4. Any limit or switch: add a getter to `AppConfig` for its config key; do not hard-code it.
5. Unit tests in `test/` for anything that maps or decides.

## Tests

```
flutter test
```

See [test/README.md](test/README.md).
