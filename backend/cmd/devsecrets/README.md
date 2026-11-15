# cmd/devsecrets

`make dev-secrets` writes `backend/.secrets.local.json` (gitignored, mode 0600) with **random**
JWT signing keys and PII encryption/HMAC keys for `ENV=local`. Shape:
[`.secrets.local.example.json`](../../.secrets.local.example.json).

It never writes real provider credentials: locally SMS, DigiLocker, payments and stores run
as stub / mock providers. Re-running with `-force` makes existing local encrypted data unreadable.
