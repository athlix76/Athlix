# tokens

Rotating refresh tokens in `refresh_tokens` (owned by the `auth` module) for app users
(`subject_type = 'user'`) and admins (`'admin'`). Used by `auth`, `user` (revoke on delete /
suspend) and `admin`.

- Only `SHA-256(token)` is stored; the raw token is returned once from `Issue` / `Rotate`.
- `Rotate` runs in one transaction with the row locked. Re-using an already rotated token
  revokes the whole family (theft detection). `expiry` decides sliding or fixed sessions and
  `allow` can refuse a subject (deleted / suspended user, disabled admin).
- `RevokeFamilyOf` (logout of one session), `RevokeAll`, `Cleanup` (used by the
  `auth.cleanup_tokens` job).

To change safely: keep every lookup by hash, keep `subject_type` in every query (a user token
must never refresh an admin session), and never log a token. Tests: `tokens_test.go` (sqlmock).
