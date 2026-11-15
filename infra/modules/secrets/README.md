# modules/secrets

Empty Secrets Manager containers under `athlix/<env>/`. Terraform creates the names;
values are put in by hand with `aws secretsmanager put-secret-value` (see
docs/aws-setup.md). The only exception is `db`, whose value the rds module writes.

| Short name | Needed | Holds |
|---|---|---|
| `db` | always | written by Terraform (rds module) |
| `jwt` | always | token signing keys |
| `pii-keys` | always | PII encryption keys |
| `msg91` | when `auth.sms_provider = msg91` | SMS/OTP credentials |
| `razorpay` | when `listing.payment_provider = razorpay` | payment keys and webhook secret |
| `digilocker` | when `verification.provider = digilocker_direct` | DigiLocker client credentials |
| `kyc-aggregator` | when `verification.provider = aggregator` | KYC partner credentials |
| `fcm` | for push notifications | Firebase service account JSON for push |
| `google-play` | when selling on Android | Play billing service account JSON |
| `app-store` | when selling on iOS | App Store Server API key |

The JSON shape of each is in the secret's description and in `docs/security.md`.

## Outputs

`arns` and `names` (maps by short name), `prefix`.

## Changing it safely

Never add an `aws_secretsmanager_secret_version` with a value here, and never put a
value in tfvars. A new secret: add it to `locals.secrets`, then to `function_secrets` in
`modules/stack`. With a recovery window of 0 (dev) a destroy deletes secrets at once.
