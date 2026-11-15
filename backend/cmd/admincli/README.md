# cmd/admincli

Operator command line for the few admin tasks that have no API: creating the first admin,
resetting an admin's credentials, and bulk-loading districts. It builds the normal `app.Deps`
(`config.LoadDBOnly()` + `app.Bootstrap`), so it talks to MySQL and Secrets Manager exactly like
the API does.

## Commands

| Command | What it does |
|---|---|
| `create-admin -email <email> -name <name> [-role <role>]` | Creates an admin (`-role` defaults to `super_admin`; must be an existing `admin_roles.name`). Prints a one-time password and the TOTP `otpauth://` URI. |
| `reset-credentials -email <email>` | Replaces the admin's password and TOTP secret and revokes all their sessions. Prints the new one-time credentials. |
| `import-districts -file <path.csv>` | Upserts `districts` from an LGD CSV (see below) via `locationidentity.ImportDistricts` and prints the `ImportResult` JSON. |

Make targets: `make create-admin EMAIL=you@example.com NAME="You" ROLE=super_admin` and
`make import-districts FILE=lgd.csv`.

Exit codes: `0` ok, `1` runtime error (bootstrap, database, validation, file), `2` usage error.

## Credentials output

The temporary password and the TOTP URI are written **once, to stdout only**. They are never
logged, never written to stderr and never stored in plain text (the password is bcrypt-hashed,
the TOTP secret is encrypted with the PII key). Run the command in a private terminal, put the
URI into an authenticator app straight away, and do not paste either value into chat, tickets
or shell history files. Lost them? Run `reset-credentials`.

## Configuration

`config.LoadDBOnly()` needs the same environment as the API except that `REDIS_ADDR` and
`MEDIA_BUCKET` are optional (without `REDIS_ADDR`, Bootstrap starts an in-process Redis, which
is fine for this tool). Secrets (db, pii-keys, jwt) come from Secrets Manager outside local,
from `.secrets.local.json` locally.

Reaching RDS through an SSH tunnel or bastion: set `DB_HOST_OVERRIDE=host[:port]`; Bootstrap
applies it to the host from the `db` secret (user and password still come from the secret):

```sh
ENV=prod DB_HOST_OVERRIDE=127.0.0.1:13306 go run ./cmd/admincli create-admin -email ops@example.com -name "Ops"
```

## District CSV format

```
state_code,district_name,lgd_code[,aliases]
TN,Thanjavur,600,Tanjore;Tanjavur
TN,Coimbatore,,Kovai
```

- `state_code` is `states.code` (e.g. `TN`); unknown codes are skipped and reported.
- `lgd_code` is optional; a row matches an existing district by `lgd_code`, else by
  (state, name); a match is updated, otherwise a new active district is inserted.
- `aliases` is optional and `;`-separated (used when matching DigiLocker address districts).
- A header row is allowed. Bad rows are listed in `skippedLines` with their line number. The
  whole import is one transaction: a database error rolls everything back.

## Files

- `main.go`: flag parsing and the three subcommands (`run` is the testable entry point).
- `main_test.go`: tests with `testutil.Deps` (sqlmock, no real database).
- `testdata/`: the small CSV fixture used by the import test.

## Changing it safely

- Keep business logic in the modules (`admin`, `locationidentity`); this command only parses
  flags, calls exported functions and prints results.
- Never print credentials anywhere but stdout, and never add logging of flag values.
- New subcommands: add a `case` in `run`, document it in the table above, add a test.
- Actions here are not written to `admin_audit_log` (there is no acting admin id); record the
  operator and reason in the ops log or ticket.
