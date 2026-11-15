# lib/features

One folder per area of the panel. Each has `data/` (API + models), `application/`
(providers, pure logic) and `presentation/` (screens), and a README listing its screens,
endpoints and permissions.

| Feature | What it does |
|---|---|
| `admins/` | Admin users and roles. |
| `ads/` | Ad campaigns: create, edit, pause / activate / end, daily stats. |
| `audit/` | Read-only audit log. |
| `auth/` | Sign-in, session and sign-out. |
| `broadcast/` | One notification to an audience (roles, sport, state). |
| `communities/` | Communities list and removal. |
| `config/` | Config editor: grouped by category, typed inputs, validation, diff before save, history. |
| `dashboard/` | Headline counts for operations. |
| `districts/` | Districts master list (names, LGD codes, aliases used to match DigiLocker addresses). |
| `events/` | Event review queue and event detail, including category and location eligibility. |
| `fee_rules/` | Listing fee rules with a live preview. |
| `location_review/` | Location identity review queue (tournament eligibility): proof viewer, approve, reject, revoke. |
| `moderation/` | Moderation action history, manual actions, and reading reported conversations. |
| `organiser_steps/` | Steps an organiser completes before approval. |
| `organisers/` | Organiser applications queue and review. |
| `payments/` | Listing-fee payments and refunds. |
| `plans/` | Premium plans and their benefits. |
| `policies/` | Policy documents (terms, privacy, location identity notice, ...) and their versions. |
| `reports/` | User reports queue and resolution. |
| `sports/` | Sports, positions and stat templates. |
| `subscriptions/` | Subscriptions list and manual grants. |
| `users/` | Find users, view one, suspend and unsuspend. |
| `verification_requirements/` | Which verification each role needs, and what it gates. |

## Adding a feature

See "Adding a screen" in `../../README.md`.

## Changing it safely

- A feature may import `core/` and another feature's `application/` or `data/` (for example the
  sports picker), never another feature's private widgets.
- Keep each feature README in step with its endpoints and permissions.
