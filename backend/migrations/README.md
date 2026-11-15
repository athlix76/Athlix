# backend/migrations

Numbered SQL files, applied once each by `cmd/migrate` (embedded with `embed.go`).

| File | Tables |
|---|---|
| 0001_users_auth.sql | users, roles, refresh tokens, guardian consent, identity verification, media, settings, policy acceptance |
| 0002_sports_profiles.sql | sports catalogue, profiles, stats, achievements, follows, blocks, profile views, organiser verification, consultations |
| 0003_events_listing.sql | events, registrations, listing fee rules, listing orders, payment webhook log |
| 0004_social_chat.sql | posts, likes, comments, conversations, members, messages, notifications, device tokens |
| 0005_billing_moderation_admin.sql | premium plans/benefits, subscriptions, ads, reports, moderation, admin users/roles/audit |
| 0006_config.sql | app_config, config_audit, policy documents, verification requirements, organiser steps |
| 0007_seed.sql | default config values, sports, positions, stat templates, fee rule, plans, policies, admin roles |
| 0008_location_identity.sql | states, districts, user location identities; event eligibility and category columns; registration qualification columns; location config and notice |
| 0009_config_keys.sql | config keys that were code-only defaults: `ads.max_per_request`, `auth.otp.max_per_ip_per_hour`, `media.pending_cleanup_hours` |
| 0010_admin_security.sql | `admin_login_events`; config `admin.login.max_per_ip`, `admin.login.window_minutes`, `admin.password_min_length`, `admin.login_events_retention_days`, `verification.subject_hash_retention_days` |
| 0011_events_ads_moderation.sql | `reports.target_type` gains `community`; config `moderation.report_rate_limit_per_hour`, `feed.comment_max_length` |
| 0012_social.sql | `conversations.removal_reason`, `community_ownership_transfers`; config `premium.restore_max_purchases` |
| 0013_location.sql | `user_location_identities.review_note` |

Rules: forward-only; one change per new file (`0014_<what>.sql` next); no foreign keys; every
filtered column indexed; PII columns follow the `_enc/_hash/_last4` pattern; update
docs/data-model.md (regenerate) and docs/config-system.md for new config keys.
