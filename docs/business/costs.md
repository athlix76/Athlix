# Running costs

> **Rough estimates for budgeting, not quotes.** Cloud cost depends on **usage and instance
> sizes** (requests, messages, stored media, data transfer, log volume, how many hours a
> database runs), **not on a price per user**. Two apps with the same user count can differ by
> 3× depending on how active users are. Re-price with the
> [AWS Pricing Calculator](https://calculator.aws/) (region **Asia Pacific (Mumbai)
> ap-south-1**) and the vendors' current price pages before using these numbers.

Exchange rate assumed: **₹88 = US$1**. AWS prices are on-demand, before GST (18 % GST applies
to AWS invoices in India; claimable as input tax credit once GST registered).

## Assumptions per tier

| | Dev / staging | ~10,000 users | ~1,00,000 users |
|---|---|---|---|
| Daily active users | team only | ~2,000 | ~20,000 |
| API requests / month | < 1 M | ~3 M | ~30 M |
| WebSocket messages / month | small | ~2 M | ~20 M |
| Media stored (S3) | < 5 GB | ~50 GB | ~500 GB |
| Data transfer out (images) | < 5 GB | ~100 GB | ~1 TB |
| Logs ingested | ~2 GB | ~10 GB | ~80 GB |
| New verifications / month | test only (mock) | ~1,500 | ~10,000 |
| OTP SMS / month | test only (stub) | ~4,000 | ~30,000 |

Architecture as in [architecture.md](../architecture.md): Lambda (arm64) behind API Gateway
HTTP + WebSocket APIs, RDS MySQL 8, ElastiCache Redis, S3 + CloudFront, SQS, one NAT gateway,
Secrets Manager.

## AWS (monthly, USD)

| Service | Dev | 10k users | 1 lakh users | Notes |
|---|---|---|---|---|
| RDS MySQL | 16 (db.t4g.micro, single-AZ, 20 GB) | 65 (db.t4g.small, Multi-AZ, 50 GB) | 155 (db.t4g.medium, Multi-AZ, 200 GB) | The biggest lever. Multi-AZ doubles instance cost; worth it in prod |
| ElastiCache Redis | 12 (cache.t4g.micro) | 12 (cache.t4g.micro) | 50 (cache.t4g.small + replica) | OTP, rate limits, WS connections |
| NAT gateway | 41 | 42 | 52 | Fixed ~$41/month + per GB. Dev can use a NAT instance (~$3) |
| Lambda (api, ws, worker) | 0 to 2 (free tier) | 5 | 36 | arm64, 512 MB, ~100 ms per request |
| API Gateway HTTP API | 0 to 1 | 3 | 33 | ~$1.1 per million requests |
| API Gateway WebSocket | 0 to 1 | 3 | 29 | messages + connection minutes |
| S3 + CloudFront | 1 | 13 | 127 | Egress to India ~ $0.11/GB dominates; compress images |
| CloudWatch (logs, metrics, alarms) | 2 | 10 | 65 | Keep logs lean; 14 to 30 day retention |
| Secrets Manager + KMS | 5 | 5 | 6 | ~10 secrets × $0.40 |
| VPC interface endpoints | 15 (one AZ) | 30 (two AZs) | 30 (two AZs) | Secrets Manager and SQS endpoints (see `infra/modules/network`), ~$7.3 per endpoint per AZ plus ~$0.01 per GB processed; keeps that traffic off the NAT |
| SQS, EventBridge Scheduler | 0 | 0 to 1 | 1 to 2 | Mostly free tier |
| WAF, backups, misc | 0 | 5 | 35 | WAF optional at 10k; snapshots beyond the free allowance |
| **AWS total (USD)** | **≈ 94** | **≈ 195** | **≈ 620** | |
| **AWS total (INR)** | **≈ ₹8,300** | **≈ ₹17,200** | **≈ ₹54,500** | |

Ways to cut cost without changing the design: stop the dev RDS at night (or run dev on
`docker compose` locally for ₹0), use a NAT instance in dev, Savings Plans / reserved RDS once
usage is stable (~30 to 40 % off), image resizing before upload, shorter log retention.

## Third-party services (monthly, INR)

| Service | Dev | 10k users | 1 lakh users | Basis |
|---|---|---|---|---|
| DigiLocker verification | 0 (mock) | ~₹7,500 | ~₹50,000 | Via an aggregator ~₹3 to ₹15 per verification (₹5 assumed); direct DigiLocker partner access may be cheaper but needs onboarding |
| SMS OTP (MSG91 + DLT) | 0 (stub) | ~₹1,000 | ~₹7,500 | ~₹0.20 to ₹0.30 per SMS; refresh tokens last `auth.refresh_token_ttl_days` (60), so most OTPs are sign-ups and new devices |
| Razorpay | 0 | ~₹800 | ~₹8,000 | ~2.36 % of listing fees charged (see [revenue-model.md](revenue-model.md)); scales with revenue |
| App stores | ~₹725 | ~₹725 | ~₹725 | Apple Developer US$99 / year; Google Play US$25 one-time. Store commission (15 %) is netted from premium revenue |
| Firebase Cloud Messaging | 0 | 0 | 0 | Free |
| Cloudflare (DNS, Access) | 0 | 0 | 0 to ₹1,800 | Free plan; Access free up to 50 users; Pro US$20 optional |
| Domain + business email | ~₹550 | ~₹550 | ~₹1,000 | Domain ~₹1,200 / year; email ~₹150 per mailbox per month |
| Error monitoring (optional) | 0 | 0 | 0 to ₹2,500 | Free tiers are enough at 10k |
| **Third-party total** | **≈ ₹1,300** | **≈ ₹10,600** | **≈ ₹68,000 to ₹72,000** | |

## All-in monthly running cost

| | Dev | 10k users | 1 lakh users |
|---|---|---|---|
| AWS | ₹8,300 (US$94) | ₹17,200 (US$195) | ₹54,500 (US$620) |
| Third-party | ₹1,300 (US$15) | ₹10,600 (US$120) | ₹70,000 (US$795) |
| **Total** | **≈ ₹9,600 (US$109)** | **≈ ₹27,800 (US$315)** | **≈ ₹1.25 lakh (US$1,415)** |

At 1 lakh users the **verification cost** is the largest single line and is paid per new
user, before they bring any revenue. Track it per month and negotiate volume pricing (or move
to direct DigiLocker access) early.

## Not included

- Salaries, contractors, marketing, office.
- One-time setup: company incorporation (typically ₹7,000 to ₹15,000 through a service
  provider plus government fees), trademark filing (₹4,500 per class for individuals, startups
  and small enterprises; ₹9,000 otherwise), lawyer review of policies (widely variable, budget
  ₹30,000 to ₹1.5 lakh), penetration test.
- Recurring professional fees: CA for GST / TDS returns and ROC filings (~₹2,000 to ₹5,000 a
  month plus annual audit).
- GST on all of the above.

Author: Atindraa.
