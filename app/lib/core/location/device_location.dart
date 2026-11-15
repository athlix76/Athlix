import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:geolocator/geolocator.dart';

/// A coordinate pair. Kept in memory only: never written to storage, never
/// logged, sent only as `lat`/`lng` query params (docs/security.md).
class LatLng {
  const LatLng(this.lat, this.lng);

  final double lat;
  final double lng;

  /// Rounded to ~1 km: enough for a radius filter, less precise than a GPS fix.
  LatLng coarse() => LatLng(_round(lat), _round(lng));

  static double _round(double v) => (v * 100).roundToDouble() / 100;

  @override
  bool operator ==(Object other) => other is LatLng && other.lat == lat && other.lng == lng;

  @override
  int get hashCode => Object.hash(lat, lng);

  @override
  String toString() => 'LatLng(<redacted>)';
}

enum LocationAccess { unknown, granted, denied }

/// Asks for permission and reads the current position (low accuracy).
class DeviceLocation {
  const DeviceLocation();

  /// Null when the service is off or permission is refused.
  Future<LatLng?> current() async {
    if (!await Geolocator.isLocationServiceEnabled()) return null;
    var perm = await Geolocator.checkPermission();
    if (perm == LocationPermission.denied) perm = await Geolocator.requestPermission();
    if (perm == LocationPermission.denied || perm == LocationPermission.deniedForever) return null;
    final p = await Geolocator.getCurrentPosition(
      locationSettings: const LocationSettings(accuracy: LocationAccuracy.low),
    );
    return LatLng(p.latitude, p.longitude).coarse();
  }
}

final deviceLocationProvider = Provider<DeviceLocation>((ref) => const DeviceLocation());

/// Whether the user allowed location this run. `denied` hides the distance filter.
final locationAccessProvider = StateProvider<LocationAccess>((ref) => LocationAccess.unknown);
