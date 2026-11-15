# Product overview

## What Athlix is

Athlix is a sports community and tournament platform for India. It gives players, coaches and
tournament organisers one place to build a verified sports profile, talk to each other, and
find, list and join tournaments, trials, camps and matches.

**Goals (from the concept document):**

- Help athletes connect with other athletes, coaches and the wider sports community.
- Make tournament and event discovery, registration and participation easier.
- Help coaches discover and assess players through verified profiles, achievements and
  performance statistics.
- Earn revenue in ways that do not require Athlix to hold anyone's money (see
  [revenue-model.md](revenue-model.md)).

**Target scale:** 10,000 to 1,00,000 (1 lakh) users in the first phase. The architecture and
the cost estimates in [costs.md](costs.md) are sized for that range.

## Who uses it

| User | Wants | Main features |
|---|---|---|
| **Player** | Be seen, find events, connect | Profile with sport, position, achievements and sport-specific stats; event discovery and registration; follow, DM, communities; feed |
| **Coach** | Find and assess talent, offer coaching | People search with sport / role / location filters; profile review; DMs; consultation listing (paid directly to the coach) |
| **Organiser** | Fill a tournament, manage registrations | Organiser verification; create and list events; registration list, waitlist and "mark paid"; event group chat |
| **Parent / guardian** | Keep a minor safe | Gives OTP consent before a minor can use the app; minor safety rules apply automatically |
| **Athlix operations** | Run a safe, trusted platform | Admin panel: review organisers and events, refunds, reports and moderation, config, plans, ads |

One person can hold several roles (a coach who also organises), with one primary role.

## Trust and safety by design

- **Everyone verifies with DigiLocker** before using the app (configurable per role in
  `verification_requirements`). Athlix keeps only the **last 4 Aadhaar digits**, the encrypted
  name and date of birth, and the verified gender. The full Aadhaar number, the e-Aadhaar XML /
  PDF and the Aadhaar photo are **never stored**. The concept document asked for exactly this:
  "establish trust without collecting more personal information than necessary".
- **Minors are allowed with parent consent.** A user below `guardian.minor_age_years`
  (default 18) needs a guardian's OTP consent. Minors get safer defaults: limited DMs
  (`minor.dm_policy`, default mutual follow), followers-only profile, limited search
  results, **no ads**. Users below `auth.min_age_years` (default 10) are refused.
- **Verified badges say what was checked:** identity (DigiLocker), organiser approval, and the
  Coach Pro badge (a paid plan, not a credential check).
- **Report, block, moderation:** every user, event, post, comment, message and conversation can
  be reported; content auto-hides at `moderation.auto_hide_report_threshold` reports pending
  review; `underage_safety` reports are reviewed first.
- **Location proofs keep the district only.** No address is stored; proof documents are seen
  only by reviewers through short audited links and are deleted
  `location.proof_retention_days` (default 30) after review.
- **Organisers are reviewed** (checklist from `organiser_verification_steps`) and, by default,
  every paid listing is reviewed before it goes live (`event.require_admin_approval`).

## Features

| Area | What the user gets | Concept section |
|---|---|---|
| Sign-up and login | Mobile OTP; choose player / coach / organiser; DigiLocker; guardian consent for minors; profile setup with sport and position | 2 |
| Home and feed | Posts with photos from people you follow, and a discover feed (your sports first) | 3 |
| Messages | 1:1 DMs, community groups, automatic group chat per published event; real-time over WebSocket with push when offline | 3 |
| Events | Organisers create and list events; players browse with filters (sport, state/district/city, date, age, category, level, fee), view details, register, join waitlists | 3, 8 |
| Location eligibility | Organisers mark an event `government` or `private` and can limit it to players native to, or living in, a district or state. Players get a native identity automatically from DigiLocker (district only) or upload a proof that an admin reviews; residence expires and is renewed. The feed shows only events you can enter (with "Show all" and a "Verify now" prompt), and the server re-checks on payment details and registration. If an identity later lapses, the registration is flagged for the organiser, never cancelled. See [location-eligibility.md](../flows/location-eligibility.md) | New (not in the concept document) |
| Search | Players and coaches by sport, role, position, location; advanced filters (age, achievement level) on premium | 3 |
| Notifications | Follows, messages, event updates, registration updates, reminders | 3 |
| Profile | Photo, bio, sport, role, location; achievements with certificates; **sport-specific stat templates**; followers / following; privacy and verification status | 3, 8 |
| Privacy controls | Profile and stats visibility, who may DM you, show in search | 8 |
| Organiser dashboard | My events, registrations, waitlist, payment marking, listing payment status | 8 |
| Coach consultations | Coach lists an offer; players send requests; the coach accepts and they chat | 4, 5 |
| Premium | Player Plus, Coach Pro, Organiser Pro (in-app purchase) | 5 |
| Help | Report from any screen; grievance officer contact (`app.grievance_officer`) in the app | 8 |

Sports at launch (seed, editable in the admin panel): cricket, football, athletics,
volleyball, basketball, badminton, kabaddi, hockey, tennis and table tennis, each with its own
positions and stat template.

## User journeys

Adapted from the concept document. The payment steps differ on purpose: see
[revenue-model.md](revenue-model.md#what-changed-from-the-concept-document).

**Player:** sign up with mobile OTP → choose *player* → DigiLocker → (minor: parent consent) →
pick sport and position, complete profile → follow players and coaches, join communities →
find an event → register (or join the waitlist) → see the organiser's contact and pay the
organiser directly → get reminders and event group chat → add achievements and stats.

**Coach:** sign up → choose *coach* → DigiLocker → select sports and coaching roles, years
coaching → search and review player profiles → follow and message players → (optionally) list
a consultation offer and accept requests.

**Organiser:** sign up → choose *organiser* → DigiLocker → complete organiser verification
(checklist) → admin approves → create an event with rules, venue, dates, eligibility and entry
fee → see the listing fee quote → pay the listing fee through Razorpay → admin review →
published → receive registrations → collect entry fees directly → mark players as paid →
conduct the event → event completes automatically after `endsAt`.

Diagrams of each journey: [../flows/](../flows/README.md).

## First launch scope

The concept document asks to prove the core value first. The first release includes:

1. Reliable registration, DigiLocker verification and profile creation.
2. Trust indicators (verified badge, organiser approval) and report / block.
3. People search and messaging.
4. Event creation, listing fee payment, discovery and registration.
5. Notifications for messages and event updates.
6. A transparent listing fee and refund policy shown before payment.

Built but **switched off at launch** (turn on from the admin Config screen when there are
active users): premium plans (`premium.enabled = false`) and ads (`ads.enabled = false`).

## Later (not built)

- Online entry-fee collection with split payouts (only through a licensed payment aggregator
  with marketplace support, after legal advice; see [compliance.md](compliance.md#rbi-payment-aggregator-rules)).
- Organiser completion evidence and payout statements (only needed if Athlix ever handles
  entry fees).
- In-app consultation booking and payment.
- Dispute / help centre beyond reports and the grievance officer.
- Fixtures, scores and live results for tournaments.
- Regional languages.

Author: Atindraa.
