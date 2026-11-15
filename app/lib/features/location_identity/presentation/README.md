# location_identity/presentation

`location_identity_screen.dart` (status cards), `location_identity_submit_screen.dart` (notice, state / district, proof type, guardian proof, masked-Aadhaar warning, file), `geo_pickers.dart`, `eligibility_chip.dart`, `not_eligible_sheet.dart`.

## Changing it safely

Screens show server messages via `errorMessage`; never log or print user data.
