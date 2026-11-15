# test/features

Tests for pure logic inside features: config value codec, fee formula mirror, proof link
expiry. Widget tests are not used; keep logic in `application/` so it can be tested here.

## Changing it safely

- One test file per logic file, named after it.
- Fixture numbers (fees, limits) stay in the tests; the app reads real values from the backend.
