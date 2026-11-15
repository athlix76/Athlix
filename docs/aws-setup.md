# AWS setup

A step-by-step guide to running Athlix on AWS, written for someone who has not used AWS
before. Follow it top to bottom for the first deploy of **dev**; the prod differences are
in [section 14](#14-prod-differences). The Terraform itself is described in
[infra/README.md](../infra/README.md).

Everything runs in **ap-south-1 (Mumbai)**. Everything below uses placeholders such as
`<account_id>`, `<profile>` or `<value>`: replace them, and never paste a real secret into
this file, a tfvars file, a ticket or a chat.

Contents:

1. [Create the AWS account](#1-create-the-aws-account)
2. [Set a budget alarm first](#2-set-a-budget-alarm-first)
3. [Create your everyday user (IAM Identity Center)](#3-create-your-everyday-user-iam-identity-center)
4. [Install the tools](#4-install-the-tools)
5. [Bootstrap Terraform state](#5-bootstrap-terraform-state)
6. [Build the Lambda zips](#6-build-the-lambda-zips)
7. [Create dev: init, plan, apply](#7-create-dev-init-plan-apply)
8. [Fill in the secrets](#8-fill-in-the-secrets)
9. [Run the migrations](#9-run-the-migrations)
10. [Create the first admin](#10-create-the-first-admin)
11. [Point the app and admin at the API](#11-point-the-app-and-admin-at-the-api)
12. [Optional: custom domain with ACM](#12-optional-custom-domain-with-acm)
13. [Partner accounts and where their keys go](#13-partner-accounts-and-where-their-keys-go)
14. [Prod differences](#14-prod-differences)
15. [Deploy routine](#15-deploy-routine)
16. [Rollback](#16-rollback)
17. [Teardown](#17-teardown)
18. [What it costs](#18-what-it-costs)

---

## 1. Create the AWS account

1. Go to <https://aws.amazon.com/> and choose **Create an AWS Account**. Use a shared
   mailbox the company controls (for example `aws-root@<your-domain>`), not a personal
   address: whoever controls that mailbox controls the account.
2. Choose the **Business** account type, enter the company's legal details and a card.
   Indian accounts are billed by Amazon Web Services India Private Limited (AISPL); add
   the company GSTIN under **Billing > Tax settings** to claim input tax credit.
3. Choose the **Basic (free)** support plan.
4. Sign in as the **root user** and immediately turn on MFA: account menu (top right) >
   **Security credentials** > **Assign MFA device**. Use an authenticator app or a hardware
   key. Store the root password and MFA recovery in the company password manager.
5. On the same page, check that the root user has **no access keys**. Never create any.
6. Under **Account** > **IAM user and role access to Billing information**, choose
   **Activate**, so your everyday user (step 3) can see bills and budgets.

From now on the root user is used only for the few tasks that require it (closing the
account, changing the support plan). Everything else is done with the user from step 3.

Optional but recommended once you have prod: create a separate account for prod with
**AWS Organizations**, so a mistake in dev can never touch prod data. This guide works
the same with one or two accounts.

## 2. Set a budget alarm first

Before creating anything that costs money:

1. Open **Billing and Cost Management** > **Budgets** > **Create budget**.
2. Choose **Use a template** > **Zero spend budget**, enter an alert email. You will get
   an email the moment anything costs more than $0.01, which confirms alerts reach you.
3. Create a second budget: template **Monthly cost budget**, amount **$150** for dev
   (raise it when prod goes live; see [section 18](#18-what-it-costs)), same email.

Terraform also creates per-environment budgets later (`monthly_budget_usd`), but these two
protect you before Terraform exists.

## 3. Create your everyday user (IAM Identity Center)

You will never create IAM users or access keys. People sign in through IAM Identity
Center and get short-lived credentials.

1. Open **IAM Identity Center** (search for it in the console), make sure the region at
   the top right is **Asia Pacific (Mumbai)**, and choose **Enable**.
2. **Users** > **Add user**: your work email, first and last name. Accept the invitation
   email and set a password and MFA.
3. **Permission sets** > **Create permission set** > **Predefined** >
   **AdministratorAccess**, session duration 8 hours. (Later, give people who only read
   logs a narrower set such as **ViewOnlyAccess**.)
4. **AWS accounts** > select the account > **Assign users or groups** > your user >
   the **AdministratorAccess** permission set.
5. Note the **AWS access portal URL** on the Identity Center dashboard
   (`https://<something>.awsapps.com/start`).

Now on your computer (after installing the CLI in step 4):

```bash
aws configure sso
# SSO session name: athlix
# SSO start URL:    https://<something>.awsapps.com/start
# SSO region:       ap-south-1
# then pick the account and AdministratorAccess; default region ap-south-1; profile name e.g. athlix-dev

aws sso login --profile athlix-dev
export AWS_PROFILE=athlix-dev
aws sts get-caller-identity          # shows the account id and your SSO role
```

Every later command in this guide assumes `AWS_PROFILE` is set and `aws sso login` is
fresh (sessions last up to 8 hours).

## 4. Install the tools

| Tool | Version | Install |
|---|---|---|
| AWS CLI | v2 | <https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html> |
| Session Manager plugin | latest | <https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html> (needed for the admin tunnel) |
| Terraform | 1.6 or newer (CI uses 1.9.8) | <https://developer.hashicorp.com/terraform/install> |
| Go | the version in `backend/go.mod` | <https://go.dev/dl/> |
| make, zip, openssl, jq | any | macOS: `brew install jq` (the rest ship with the OS / Xcode tools); Ubuntu: `sudo apt install make zip openssl jq` |

Check:

```bash
aws --version && session-manager-plugin --version && terraform -version && go version && jq --version
```

## 5. Bootstrap Terraform state

Terraform records what it created in a **state** file. For dev and prod it lives in an S3
bucket with a DynamoDB lock so two people cannot apply at once. The bucket is created by a
small separate root, once per account:

```bash
cd infra/bootstrap
terraform init
terraform apply -var 'github_repo=<owner>/<repo>'   # the GitHub repo, for the CI plan role; omit to skip
terraform output
```

Type `yes` when asked. Note the outputs:

- `state_bucket` (`athlix-tfstate-<account_id>`) and `lock_table` (`athlix-tfstate-lock`)
- `plan_role_arn`: for the CI workflow (step 15)

This also creates the account-level role API Gateway needs to write WebSocket logs.
The bootstrap state stays in `infra/bootstrap/terraform.tfstate` on your machine
(gitignored); keep a copy in the password manager. See
[infra/bootstrap/README.md](../infra/bootstrap/README.md).

## 6. Build the Lambda zips

```bash
make -C backend build-lambda
ls backend/dist/            # api.zip  migrate.zip  worker.zip  ws.zip
```

Terraform uploads these files; it reads them during `plan`, so build before every plan.

## 7. Create dev: init, plan, apply

```bash
cd infra/envs/dev
cp backend.hcl.example backend.hcl                # set bucket = "athlix-tfstate-<account_id>"
cp terraform.tfvars.example terraform.tfvars      # set alert_emails; leave the rest for now
terraform init -backend-config=backend.hcl
terraform plan -out=tfplan
terraform apply tfplan
```

The first apply takes about 15 to 20 minutes (the database and Redis are the slow part).
It creates the VPC, MySQL, Redis, the media bucket, the queue, the four Lambdas, both API
Gateways, the scheduled jobs, alarms and **empty** secrets. It writes the `db` secret
itself and runs the migrations (step 9).

Then:

- Each address in `alert_emails` receives an **AWS Notification - Subscription
  Confirmation** email. Click the link, or alarms will not reach you.
- `terraform output` prints the URLs and names used below. None of them is a secret.

The api, ws and worker functions will fail until step 8 is done, because they need the
`jwt` and `pii-keys` secrets at startup.

## 8. Fill in the secrets

Terraform creates every secret under `athlix/dev/` with no value. You put the values in
with the CLI. The shapes are in [docs/security.md](security.md#secrets) and in each
secret's description in the console.

Rules while doing this:

- The commands below generate random keys inside the command, so they never appear on
  screen or in shell history.
- For partner keys, `read -rs` asks for the value without echoing it.
- Never write a value into a file inside the repo, a tfvars file, a ticket or a chat.
  Share a secret's **name** (`athlix/dev/razorpay`), never its value.

`db` is already filled by Terraform. Do not change it by hand.

**jwt** (required):

```bash
aws secretsmanager put-secret-value --secret-id athlix/dev/jwt --secret-string \
  "{\"userSigningKey\":\"$(openssl rand -base64 32)\",\"adminSigningKey\":\"$(openssl rand -base64 32)\"}"
```

**pii-keys** (required). This key encrypts personal data in the database. Losing it
means that data can never be read again; it stays only in Secrets Manager (prod keeps
deleted secrets recoverable for 30 days).

```bash
aws secretsmanager put-secret-value --secret-id athlix/dev/pii-keys --secret-string \
  "{\"current\":1,\"keys\":{\"1\":\"$(openssl rand -base64 32)\"},\"hmacKey\":\"$(openssl rand -base64 32)\"}"
```

**Partner secrets** (fill the ones you use; see [section 13](#13-partner-accounts-and-where-their-keys-go)).
Example for MSG91:

```bash
read -rs MSG91_AUTH_KEY && export MSG91_AUTH_KEY      # paste, press Enter (nothing is shown)
aws secretsmanager put-secret-value --secret-id athlix/dev/msg91 --secret-string \
  "$(jq -cn --arg a "$MSG91_AUTH_KEY" '{authKey:$a, otpTemplateId:"<template_id>", guardianTemplateId:"<template_id>", senderId:"<sender_id>"}')"
unset MSG91_AUTH_KEY
```

For a Google service-account JSON (FCM, Google Play), convert the downloaded file and
delete it straight after:

```bash
aws secretsmanager put-secret-value --secret-id athlix/dev/fcm --secret-string \
  "$(jq -c '{projectId:.project_id, clientEmail:.client_email, privateKey:.private_key}' ~/Downloads/<service-account>.json)"
rm -- ~/Downloads/<service-account>.json
```

Check what is filled, without printing any value:

```bash
for s in jwt pii-keys msg91 razorpay digilocker kyc-aggregator fcm google-play app-store; do
  printf '%-16s ' "$s"
  aws secretsmanager list-secret-version-ids --secret-id "athlix/dev/$s" \
    --query 'length(Versions)' --output text
done                                   # 0 = still empty
```

The Lambdas read secrets when they start. After changing a value, make them restart by
running `infra/scripts/deploy.sh dev` (or wait: idle instances are recycled within
hours).

## 9. Run the migrations

Nothing to do by hand: `terraform apply` invokes the `athlix-dev-migrate` Lambda whenever
its code changed, **before** it updates the other functions. If a migration fails, the
apply fails and the previous code keeps running.

To run it by hand (after fixing a failed apply, or if `run_migrations_on_apply = false`):

```bash
infra/scripts/migrate.sh dev
```

Its logs are in CloudWatch: log group `/aws/lambda/athlix-dev-migrate`.

## 10. Create the first admin

The admin CLI talks to MySQL and Redis, which have no public address. Instead of a
bastion server, you open a **Session Manager port-forward** through a tiny instance that
exists only while you need it (no public IP, no SSH, no open port).

1. In `infra/envs/dev/terraform.tfvars` set `enable_ops_tunnel = true`, then
   `terraform apply` (about 1 minute).
2. Make the database hostname point to your machine, so the CLI's connection (which
   uses the host stored in the `db` secret) goes through the tunnel:

   ```bash
   terraform -chdir=infra/envs/dev output -raw rds_address    # <rds_address>
   echo "127.0.0.1 <rds_address>" | sudo tee -a /etc/hosts
   ```

3. Terminal 1: `infra/scripts/tunnel.sh dev mysql` (leave it running).
   Terminal 2: `infra/scripts/tunnel.sh dev redis` (leave it running).
4. Terminal 3, from `backend/`, with your SSO session active:

   ```bash
   ENV=dev SECRET_PREFIX=athlix/dev AWS_REGION=ap-south-1 \
   REDIS_ADDR=127.0.0.1:6379 \
   MEDIA_BUCKET="$(terraform -chdir=../infra/envs/dev output -raw media_bucket)" \
   go run ./cmd/admincli create-admin -email "<you@company.com>" -name "<Your Name>" -role super_admin
   ```

   It reads the `db` and `pii-keys` secrets with your SSO credentials (the admin's TOTP
   secret is encrypted with this environment's key, which is why it must run against the
   real secrets). It prints a one-time password and a TOTP URI: put the URI into your
   authenticator app and the password into the password manager, then clear the
   terminal.
5. Clean up: stop both tunnels (Ctrl+C), remove the `/etc/hosts` line, set
   `enable_ops_tunnel = false` and `terraform apply`.

## 11. Point the app and admin at the API

```bash
terraform -chdir=infra/envs/dev output -raw app_api_url     # https://<id>.execute-api.ap-south-1.amazonaws.com/v1
```

Both Flutter projects take it at build time:

```bash
cd app   && flutter run   --dart-define=API_BASE_URL=<app_api_url> --dart-define=ENV=dev
cd admin && flutter build web --dart-define=API_BASE_URL=<app_api_url> --dart-define=ENV=dev
```

The chat WebSocket URL is not configured in the app: the API returns it with each chat
ticket (`ws_public_url` output). Add every origin the admin web is served from (for
example `http://localhost:<port>` while developing with `flutter run -d chrome --web-port <port>`) to `admin_origins` in
`terraform.tfvars` and apply; it controls CORS for the API and for direct uploads.

Hosting the admin web build is not part of this Terraform. Any static host works; put it
behind Cloudflare Access or an IP allowlist (see docs/security.md).

## 12. Optional: custom domain with ACM

Without this, the execute-api URLs work fine for dev.

1. In **Certificate Manager**, region **ap-south-1** (not us-east-1: these are regional
   API Gateway domains), **Request** a public certificate for `api.<domain>` and
   `ws.<domain>` (or `*.<domain>`), DNS validation.
2. Create the validation CNAME records it shows at your DNS provider and wait for
   **Issued**.
3. In `terraform.tfvars`:

   ```hcl
   certificate_arn = "<certificate_arn>"
   api_domain_name = "api.<domain>"
   ws_domain_name  = "ws.<domain>"
   ```

4. `terraform apply`, then create CNAME records:
   `api.<domain>` to the `api_custom_domain_target` output and `ws.<domain>` to
   `ws_custom_domain_target`. With Cloudflare, start with **DNS only** (grey cloud).
5. `app_api_url` now prints `https://api.<domain>/v1`; rebuild the apps with it.

## 13. Partner accounts and where their keys go

Every partner key goes **straight into Secrets Manager** with `put-secret-value` (step 8)
and nowhere else. Shapes: [docs/security.md](security.md#secrets).

| Partner | Sign up | What you get | Secret | Also configure at the partner |
|---|---|---|---|---|
| DigiLocker | Partner onboarding via API Setu / DigiLocker (needs company KYC; takes weeks) | client id, client secret, endpoints | `athlix/<env>/digilocker` | redirect URI `https://api.<domain>/v1/verification/digilocker/callback` |
| KYC aggregator (alternative to direct DigiLocker) | the aggregator's dashboard | API key + secret, webhook secret | `athlix/<env>/kyc-aggregator` | webhook URL from docs/api.md |
| Razorpay | <https://dashboard.razorpay.com/> (test mode is instant; live needs KYC) | key id, key secret; you choose a webhook secret | `athlix/<env>/razorpay` | webhook `https://api.<domain>/v1/webhooks/razorpay`, events `payment.captured`, `order.paid`, `payment.failed`, `refund.processed` |
| MSG91 | <https://msg91.com/> plus DLT registration of sender id and templates (TRAI requirement) | auth key, template ids, sender id | `athlix/<env>/msg91` | none |
| Firebase Cloud Messaging | Firebase console > project > Project settings > Service accounts > Generate new private key | service-account JSON | `athlix/<env>/fcm` (converted with jq, file deleted) | add the Android/iOS apps; the app's `google-services.json` / `GoogleService-Info.plist` are not secrets |
| Google Play | Play Console (US$25 once) > API access: service account with financial data permission; Pub/Sub topic for RTDN | service-account JSON, package name, Pub/Sub push audience and account | `athlix/<env>/google-play` | RTDN push endpoint `https://api.<domain>/v1/webhooks/google-play` |
| App Store | Apple Developer Program (US$99/year) > App Store Connect > Users and Access > Integrations > In-App Purchase key | `.p8` key, key id, issuer id | `athlix/<env>/app-store` (paste the `.p8` contents as `privateKey`, then delete the file) | Server Notifications v2 URL `https://api.<domain>/v1/webhooks/app-store` |

Use each partner's **test/sandbox** credentials in dev and live ones only in prod. Some
partners allowlist caller IPs: give them the `nat_public_ip` output (all Lambda traffic
leaves through it).

## 14. Prod differences

Prod is a separate root (`infra/envs/prod`) with its own state key, secrets
(`athlix/prod/*`) and sizes; see [infra/envs/prod/README.md](../infra/envs/prod/README.md):
Multi-AZ `db.t4g.medium` with 14-day backups and deletion protection, two Redis nodes,
endpoints in both AZs, reserved Lambda concurrency, higher throttling limits, 30-day
logs, versioned media.

Before the first prod apply:

1. Ideally use a separate prod account (step 1), with its own bootstrap (step 5).
2. Ask AWS for a higher Lambda concurrency quota: **Service Quotas** > **AWS Lambda** >
   **Concurrent executions** > request **1000**. Until it is granted, set the three
   `*_reserved_concurrency` values to `-1` in `terraform.tfvars`.
3. Move partner accounts (Razorpay, MSG91, DigiLocker, stores) to live mode and put
   their live keys into the `athlix/prod/*` secrets.
4. Generate **new** `jwt` and `pii-keys` values for prod. Never copy dev values, and never
   copy production data into dev.
5. Set a custom domain (step 12) and raise the account budget (step 2).

Then repeat steps 6 to 11 with `prod` in place of `dev`.

## 15. Deploy routine

For every release:

```bash
git pull                                   # the commit you want to release
infra/scripts/deploy.sh dev                # builds zips, plans, asks you to type "dev", applies
# check the app against dev
infra/scripts/deploy.sh prod
```

During the apply, migrations run first; the api, ws and worker functions update only if
they succeed. Lambda swaps code without downtime.

Pull requests that change `infra/**` get an automatic dev plan from
`.github/workflows/terraform-plan.yml`. It needs three repository **secrets**
(Settings > Secrets and variables > Actions): `AWS_PLAN_ROLE_ARN` (bootstrap output),
`TF_STATE_BUCKET` (bootstrap output) and `DEV_TFVARS` (contents of the dev
`terraform.tfvars`, which holds no secret values). It uses GitHub OIDC; there are no AWS
keys in GitHub. Applies stay manual.

## 16. Rollback

- **Code:** check out the previous release commit and run `infra/scripts/deploy.sh <env>`.
  Migrations are forward-only and are not undone, so every release must work with the
  schema of the next one (add columns before using them, drop them a release later).
- **A bad migration:** fix it forward with a new migration. If data was damaged, restore
  the database: **RDS** > the instance > **Actions** > **Restore to point in time**
  (within the backup retention: 1 day in dev, 14 in prod). This creates a new instance;
  compare, then copy data back or switch over with help from someone experienced.
- **Infrastructure change:** revert the Terraform commit and apply. Read the plan first:
  anything that says "must be replaced" on the database, Redis, the bucket or a secret
  needs a careful look.
- **A secret changed by mistake:** Secrets Manager keeps the previous version; restore
  it with `aws secretsmanager update-secret-version-stage` (move `AWSCURRENT` back to the
  previous version id from `list-secret-version-ids`), then redeploy.

## 17. Teardown

**Dev** can be removed completely:

```bash
cd infra/envs/dev
terraform destroy
```

The media bucket is emptied and secrets are deleted immediately (dev settings).

**Prod** is protected on purpose. Only as a deliberate, reviewed decision:

1. Export what you must keep (database snapshot, media).
2. Set `db_deletion_protection = false` in `infra/envs/prod/main.tf` and apply.
3. `terraform destroy`. A final DB snapshot is taken; the media bucket refuses to delete
   while it has objects (empty it yourself after confirming the export); secrets stay
   recoverable for 30 days.

The bootstrap state bucket and lock table have `prevent_destroy`; remove them only when
closing the account. To stop all charges, also check **Billing** for anything left over
(snapshots, Elastic IPs) and finally close the account as the root user.

## 18. What it costs

AWS bills for **usage and instance sizes**: hours a database or NAT gateway runs, requests,
messages, stored and transferred data, and log volume. There are **no per-user
licences**: the same setup costs about the same for 10 users or 10,000 until traffic
grows. Approximate on-demand prices for ap-south-1, in USD per month, before 18% GST
(at ₹88 = US$1). Re-check with the [AWS Pricing Calculator](https://calculator.aws/);
traffic assumptions are in [docs/business/costs.md](business/costs.md).

| Item | Dev (as configured) | Prod at ~10k users | Prod at ~1 lakh users | Driven by |
|---|---|---|---|---|
| RDS MySQL | 16 (db.t4g.micro, single-AZ, 20 GB) | 120 (db.t4g.medium, Multi-AZ, 50 GB) | 160 (same, ~200 GB) | instance hours, storage |
| ElastiCache Redis | 12 (1 x cache.t4g.micro) | 50 (2 x cache.t4g.small) | 50 | node hours |
| NAT gateway | 42 | 42 | 52 | fixed hourly + per GB |
| VPC interface endpoints (Secrets Manager, SQS) | 15 (1 AZ) | 30 (2 AZs) | 30 | fixed hourly per AZ |
| Lambda (api, ws, worker, migrate) | 0 to 2 | 5 | 36 | requests x duration |
| API Gateway HTTP + WebSocket | 0 to 2 | 6 | 62 | requests, messages, connection minutes |
| S3 media (+ CloudFront if on) | 1 | 13 | 127 | storage, data out |
| CloudWatch logs and alarms | 3 | 10 | 65 | log volume, alarm count |
| Secrets Manager | 4 | 4 | 6 | $0.40 per secret |
| SQS, EventBridge Scheduler | 0 | 0 to 1 | 1 to 2 | mostly free tier |
| Backups, misc | 0 | 5 | 25 | snapshots beyond the free allowance |
| Ops tunnel instance (only while on) | 0 to 3 | 0 to 3 | 0 to 3 | hours running |
| **Total (USD)** | **≈ 95** | **≈ 290** | **≈ 615** | |
| **Total (INR)** | **≈ ₹8,400** | **≈ ₹25,500** | **≈ ₹54,000** | |

Ways to spend less: at the 10k stage prod can run `db.t4g.small` and `cache.t4g.micro`
(about $85 less; change the sizes in `infra/envs/prod/main.tf`); add
`enable_interface_endpoints = false` to the stack call in `infra/envs/dev/main.tf` to
save about $15 in dev (that traffic then goes through the NAT); buy Savings Plans / reserved RDS once usage is stable (30 to 40% off); keep log
volume low. Partner and store fees (SMS, KYC, payments) are separate; see
docs/business/costs.md.
