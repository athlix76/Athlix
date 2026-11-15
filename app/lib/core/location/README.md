# core/location

`device_location.dart`: `LatLng`, `LocationAccess`, and `DeviceLocation`, which asks for permission and reads
one low-accuracy (coarse) position through geolocator, exposed as `deviceLocationProvider`.

Used only by the events "Near me" distance filter. The position is kept in memory in `EventFilters`, sent as
`lat` / `lng` on `GET /events`, and **never stored, logged or sent anywhere else**.

## Changing it safely

Do not add background location, fine accuracy, persistence or logging of coordinates. If permission is
denied, callers hide the distance UI.
