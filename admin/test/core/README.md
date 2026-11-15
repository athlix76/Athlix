# test/core

Tests for `lib/core`: permission checks and sidebar gating (`permissions_test.dart`), and API
error mapping (`api_error_test.dart`).

## Changing it safely

- A new permission in `Perm` needs no new test unless it changes gating rules; a new nav item is
  checked automatically (known permission, unique path).
- A new error code in `ErrorCodes` with special handling gets a case in `api_error_test.dart`.
