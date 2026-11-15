# otp

One-time-code helpers shared by login (`auth`) and parent consent (`guardian`).

- `NormalizeMobile` / `SMSNumber`: Indian mobile to `+91XXXXXXXXXX` and to the SMS sender form.
- `Generate(length)`: random numeric code (crypto/rand).
- `Store`: Redis challenge hashes holding only `SHA-256(scope:code)` plus non-PII extra fields
  and an attempt counter. `Check` is single use and burns the challenge at `maxAttempts`.
  `Cooldown` is a SETNX resend guard.

To change safely: never store the raw code or a raw mobile number in a challenge, keep the
compare constant-time, and keep `Check` deleting the challenge on success (single use).
Tests: `otp_test.go` (miniredis).
