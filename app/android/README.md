# app/android

The Android project is generated, not committed by hand. Create it from `app/` with:

```
flutter create --platforms=android,ios --org com.athlix .
```

Then apply the edits below. Application id: `com.athlix.athlix` (from `--org` and the package name).

## AndroidManifest.xml (`android/app/src/main/AndroidManifest.xml`)

Permissions (inside `<manifest>`):

```xml
<uses-permission android:name="android.permission.INTERNET"/>
<uses-permission android:name="android.permission.CAMERA"/>
<uses-permission android:name="android.permission.READ_MEDIA_IMAGES"/>
<uses-permission android:name="android.permission.READ_EXTERNAL_STORAGE" android:maxSdkVersion="32"/>
<uses-permission android:name="android.permission.POST_NOTIFICATIONS"/>
<!-- Distance filter only. Coarse is enough; fine location is never requested. -->
<uses-permission android:name="android.permission.ACCESS_COARSE_LOCATION"/>
```

DigiLocker return deep link (inside the main `<activity>`):

```xml
<intent-filter>
  <action android:name="android.intent.action.VIEW"/>
  <category android:name="android.intent.category.DEFAULT"/>
  <category android:name="android.intent.category.BROWSABLE"/>
  <data android:scheme="athlix" android:host="verification" android:pathPrefix="/result"/>
</intent-filter>
<meta-data android:name="flutter_deeplinking_enabled" android:value="true"/>
```

## Firebase

Download `google-services.json` for the matching environment from the Firebase console and put it at
`android/app/google-services.json`. It is gitignored; never commit it. Add the
`com.google.gms.google-services` Gradle plugin as the FlutterFire docs describe.

## Root `android/build.gradle.kts`: raise plugin compileSdk

`file_picker` 8.x pins `compileSdk 34`, but `flutter_plugin_android_lifecycle` requires
plugins that depend on it to compile against 36+, so `assembleDebug` fails in
`:file_picker:checkDebugAarMetadata`. Add this block **before** the generated
`subprojects { project.evaluationDependsOn(":app") }` block:

```kotlin
subprojects {
    afterEvaluate {
        val android = extensions.findByName("android")
        if (android is com.android.build.gradle.LibraryExtension) {
            val current = android.compileSdk ?: 0
            if (current < 36) android.compileSdk = 36
        }
    }
}
```

Needs Android SDK platform 36 installed (Android Studio > SDK Manager). Remove the block once
every plugin compiles against 36 or newer on its own.

## Debug-only cleartext (`android/app/src/debug/AndroidManifest.xml`)

The emulator reaches the local API over plain `http://10.0.2.2:8080`, which Android blocks by
default. Allow it **in the debug manifest only** (release builds keep HTTPS-only):

```xml
<application android:usesCleartextTraffic="true"/>
```

## Branding (icon, splash, name)

Generated from `app/tool/brand` (see its README). After `flutter create`, restore:

- `android:label="Athlix"` in `AndroidManifest.xml` (the template writes `athlix`).
- `mipmap-*/ic_launcher.png` (legacy), `mipmap-*/ic_launcher_foreground.png` and
  `ic_launcher_background.png`, and `mipmap-anydpi-v26/ic_launcher.xml` (adaptive icon).
- `values/colors.xml` with `athlix_navy` `#031727`.
- `drawable/launch_background.xml` and `drawable-v21/launch_background.xml`: navy plus the centred
  `@drawable/splash_logo` (`drawable-*/splash_logo.png`).
- `values/styles.xml` and `values-night/styles.xml`: `NormalTheme` window background
  `@color/athlix_navy` (no white flash before the first frame).
- `values-v31/styles.xml` **and** `values-night-v31/styles.xml` (the night qualifier outranks the
  API level): `windowSplashScreenBackground` navy, `windowSplashScreenAnimatedIcon`
  `@drawable/splash_icon_v31`.

## Emulator tips

- Typing with the Mac keyboard needs `hw.keyboard=yes` in
  `~/.android/avd/<name>.avd/config.ini` (restart the emulator after changing it).
- Run: `flutter run -d emulator-5554 --dart-define=API_BASE_URL=http://10.0.2.2:8080/v1 --dart-define=ENV=local`.

## Other

- `minSdkVersion` 23 or higher (flutter_secure_storage, firebase_messaging, geolocator).
- Razorpay needs no extra manifest entries. Its key id comes from the server per order, never from the app.
- Release signing: `android/key.properties` and keystores are gitignored. Keep them out of the repo.

## Changing it safely

Only add a permission when a feature in `lib/` uses it, and update the permission list in `app/README.md`.
