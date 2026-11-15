# app/ios

The iOS project is generated. From `app/` run:

```
flutter create --platforms=android,ios --org com.athlix .
```

Then make these edits. Bundle id: `com.athlix.athlix`.

## Info.plist (`ios/Runner/Info.plist`)

Usage strings (all required, or iOS rejects the build / crashes on first use):

```xml
<key>NSCameraUsageDescription</key>
<string>Take a photo for your profile, posts or documents.</string>
<key>NSPhotoLibraryUsageDescription</key>
<string>Choose photos for your profile, posts or documents.</string>
<key>NSLocationWhenInUseUsageDescription</key>
<string>Show events near you. Your location is not saved.</string>
```

DigiLocker return deep link:

```xml
<key>CFBundleURLTypes</key>
<array>
  <dict>
    <key>CFBundleURLSchemes</key>
    <array><string>athlix</string></array>
  </dict>
</array>
<key>FlutterDeepLinkingEnabled</key>
<true/>
```

## Push notifications

- Enable the **Push Notifications** and **Background Modes > Remote notifications** capabilities in Xcode.
- Upload the APNs key to Firebase (console only; the `.p8` file never goes in the repo).

## Firebase

Put `GoogleService-Info.plist` at `ios/Runner/GoogleService-Info.plist` (gitignored). Never commit it.

## CocoaPods, not Swift Package Manager

`app/pubspec.yaml` sets `flutter: config: enable-swift-package-manager: false`. Under SwiftPM,
Xcode 26+ refuses `SwiftyGif` (pulled in by `file_picker` → `DKPhotoGallery`) because of its
old Swift tools version, and package resolution fails. With CocoaPods the first `pod install`
downloads large pods (Firebase, Razorpay) and can take a while; on a flaky connection re-run
`cd ios && pod install` (downloaded pods are cached).

## Simulator

Xcode 27 has no separate Simulator app window (`open -a Simulator` fails; devices live in
`Xcode.app/Contents/Applications/DeviceHub.app`). `flutter run -d "iPhone 18 Pro"
--dart-define=API_BASE_URL=http://localhost:8080/v1 --dart-define=ENV=local` builds and runs,
but for clicking through screens locally the Android emulator is easier (see ../android/README.md).

## Other

- Deployment target iOS 15 or higher (the generated project uses 15.0).
- In-app purchase: enable the **In-App Purchase** capability. Product ids come from `GET /premium/plans`.

## Changing it safely

Every new permission needs a usage string here and a line in `app/README.md`.
