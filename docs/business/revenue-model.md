# Revenue model

> Every number on this page is a **default** (a seed row in the database). All of them are
> edited from the admin panel without a deploy: fee rules in `listing_fee_rules`, plans and
> prices in `premium_plans`, refund percentages in `app_config` (`listing.refund.*`). See
> [config-system.md](../config-system.md).

## The one rule: Athlix never holds money

Players pay tournament **entry fees directly to the organiser**: cash at the venue, UPI, bank
transfer, or the organiser's own payment link. Athlix shows the organiser's payment modes,
instructions and contact (`GET /events/:id/contact`), and lets the organiser mark each
registration as paid. Athlix never collects, holds, splits or refunds an entry fee, and the
code is not allowed to (repo hard rules, see [AGENTS.md](../../AGENTS.md)).

Athlix earns only from money paid **to Athlix for Athlix's own services**:

| Stream | Who pays | How it is paid | Status at launch |
|---|---|---|---|
| **Listing fee** | Organiser | Razorpay (Athlix is the merchant) | On |
| **Premium plans** | Player / coach / organiser | Google Play Billing / Apple in-app purchase | Built, off (`premium.enabled = false`) |
| **Contextual ads** | Sports brands, academies, local businesses | Invoiced directly (outside the app) | Built, off (`ads.enabled = false`) |
| Coach consultations | Player pays the **coach directly** | Outside Athlix | Athlix earns via Coach Pro (see below) |

## What changed from the concept document

The concept document proposed that players pay entry fees **through Athlix**, that Athlix
tracks the funds per event, holds the organiser's payout until the event is complete, and
releases it in two stages (**50 % after completion, 50 % after final checks**), keeping an
**event registration commission** and possibly a **platform fee on coach consultations**.

That design was replaced by the listing-fee model because:

1. **Holding other people's money is a regulated activity.** Collecting money from players on
   behalf of organisers and settling it later is payment aggregation. Under the RBI rules for
   payment aggregators, a business that holds and settles funds for third parties needs RBI
   authorisation (with a high net-worth requirement) or must use a licensed aggregator's
   marketplace / escrow product with its own contracts, KYC of every organiser, nodal account
   rules and settlement timelines. The concept document itself warned that "payment aggregation
   and escrow-like arrangements may be regulated". See
   [compliance.md](compliance.md#rbi-payment-aggregator-rules).
2. **Disputes, chargebacks and refunds of entry fees** become Athlix's problem the moment
   Athlix touches the money: cancelled or postponed events, no-show organisers, partial
   refunds. A pre-company startup cannot carry that risk.
3. **Simpler tax.** With the listing fee, Athlix invoices one service (its own) and charges
   GST on it. It is not collecting consideration for other people's supplies.
4. **It still captures value from the same activity.** The fee scales with the size of the
   tournament (entry fee × slots), so it behaves like a commission without touching the
   entry fees.

If online entry-fee collection is ever added, it must go through a licensed aggregator's
marketplace / route product, after legal advice, and these documents must be rewritten first.

## 1. Listing fee (main revenue)

The organiser pays a one-time listing fee when submitting an event for publication.

```
gross    = entryFeePaise × maxSlots
base     = entryFeePaise == 0 ? freeEventFeePaise : clamp(round(gross × percentBp / 10000), minFeePaise, maxFeePaise)
gst      = round(base × gstBp / 10000)
total    = base + gst                 (half-up rounding to the paisa)
```

**Defaults** (the seed `Default` rule in `listing_fee_rules`):

| Field | Default |
|---|---|
| `percentBp` | 500 (5 % of entry fee × max slots) |
| `minFeePaise` | 9900 (₹99) |
| `maxFeePaise` | 499900 (₹4,999) |
| `freeEventFeePaise` | 9900 (₹99 for events with no entry fee) |
| `gstBp` | 1800 (18 % GST) |

- Rules can be added per sport, per event type, or both (the most specific active rule wins),
  with effective dates, for example a launch promotion of 0 % for one sport. A quote with total
  0 is marked paid immediately.
- The quote is **snapshotted** on the order: editing a rule never changes an order already made.
- The organiser sees the full breakdown (gross, %, base, GST, total) and the refund policy
  **before** paying (`GET /listing/quote` and `POST /events/:id/listing/quote`).

**Refunds of the listing fee** (`app_config`, shown before payment with the
`listing_refund` policy document):

| Situation | Key | Default |
|---|---|---|
| Organiser cancels before anyone registers | `listing.refund.before_first_registration_percent` | 100 % |
| Organiser cancels after registrations exist | `listing.refund.after_registrations_percent` | 0 % |
| Athlix rejects the event in review | `listing.refund.on_admin_rejection_percent` | 100 % |
| Athlix removes a published event for a rule violation | (no refund) | 0 % |

Finance staff can also issue a manual refund from the admin panel (`payments.refund`).

### Worked examples (default rule)

| Case | Entry fee × slots | Gross | 5 % | After min/max | GST 18 % | **Organiser pays** |
|---|---|---|---|---|---|---|
| A. Typical tournament | ₹500 × 16 teams | ₹8,000 | ₹400 | ₹400 | ₹72.00 | **₹472.00** |
| B. Small event (min applies) | ₹100 × 10 | ₹1,000 | ₹50 | ₹99 | ₹17.82 | **₹116.82** |
| C. Large event (max applies) | ₹2,000 × 64 | ₹1,28,000 | ₹6,400 | ₹4,999 | ₹899.82 | **₹5,898.82** |
| D. Free event | ₹0 × any | ₹0 | n/a | ₹99 (free-event fee) | ₹17.82 | **₹116.82** |

What Athlix keeps from each, assuming the Razorpay standard fee of about **2 % + 18 % GST on
that fee = 2.36 % of the amount charged** (check the current Razorpay plan; some methods
differ):

| Case | Charged | GST to the government | Gateway fee (≈2.36 %) | **Athlix net** | Net as % of the players' total entry fees |
|---|---|---|---|---|---|
| A | ₹472.00 | ₹72.00 | ₹11.14 | **₹388.86** | 4.9 % of ₹8,000 |
| B | ₹116.82 | ₹17.82 | ₹2.76 | **₹96.24** | 9.6 % of ₹1,000 |
| C | ₹5,898.82 | ₹899.82 | ₹139.21 | **₹4,859.79** | 3.8 % of ₹1,28,000 |
| D | ₹116.82 | ₹17.82 | ₹2.76 | **₹96.24** | n/a |

(The GST charged by Razorpay on its own fee can usually be claimed as input tax credit once
Athlix is GST registered; it is ignored here.)

**Organiser's view (case A):** 16 teams × ₹500 = ₹8,000 collected directly by the organiser.
They pay Athlix ₹472 once (5.9 % of their collections including GST) and keep all entry fees.

## 2. Premium plans

Sold as auto-renewing subscriptions through **Google Play Billing** and **Apple in-app
purchase**, because the benefits are digital features used inside the app (store rules:
[compliance.md](compliance.md#app-store-payment-rules)). The server verifies every purchase
with the store.

| Plan (`premium_plans`) | Audience | Default price | Benefits (`premium_plan_benefits`, closed list) |
|---|---|---|---|
| Player Plus | player | ₹99 / month | `profile_boost`, `advanced_stats`, `see_profile_viewers` |
| Coach Pro | coach | ₹299 / month | `consultation_listing`, `advanced_search_filters`, `extra_communities` = 25 |
| Organiser Pro | organiser | ₹499 / month | `featured_events` = 2 per month, `extra_communities` = 25 |

**What Athlix keeps per subscriber per month** (assumes store prices are GST-inclusive and the
store's 15 % small-developer rate for subscriptions; confirm both with the current store
policies and a CA):

| Plan | Price | Less GST (18 % inclusive) | Less store fee (15 %) | **Athlix net** |
|---|---|---|---|---|
| Player Plus | ₹99 | ₹83.90 | ₹12.58 | **₹71.31** |
| Coach Pro | ₹299 | ₹253.39 | ₹38.01 | **₹215.38** |
| Organiser Pro | ₹499 | ₹422.88 | ₹63.43 | **₹359.45** |

## 3. Coach consultations

**Default (founder to confirm):** consultation fees are paid **directly by the player to the
coach**. Athlix lists the offer and the request / accept flow, then the two chat. Athlix takes
no cut of consultation fees; it earns from **Coach Pro**, whose `consultation_listing` benefit
can be made mandatory for offering consultations.

Note the seed: the `verification_requirements` row `coach` / `coach_pro_subscription` /
`offer_consultation` is `is_required = 0`, so **at launch any coach can list consultations for
free**. To make Coach Pro the price of listing consultations, set that row to required in the
admin panel (Verification requirements screen) once premium is switched on.

Taking a platform fee on consultations (as the concept document suggested) would again mean
collecting money on someone else's behalf, and person-to-person services have their own store
rules. Do not add it without legal advice.

## 4. Contextual ads

- Sponsored cards in `home_feed` (one after every `ads.feed_every_n_posts` posts, default 8),
  `events_list`, `search` and `event_detail`, labelled "Sponsored".
- Targeting is **contextual only**: placement, sport and state. No personal profiling, no
  third-party ad SDKs, no tracking.
- **Never shown to minors** (`ads.show_to_minors = false`; keep it false, see
  [compliance.md](compliance.md#childrens-data)).
- Sold directly to sports brands, equipment shops, academies and local businesses; billed by
  invoice (with GST) outside the app; the admin panel shows daily impressions and clicks per
  campaign for reporting.
- Suggested starting price: a flat monthly fee per placement and state (for example ₹3,000 to
  ₹10,000 a month for a local academy), moving to CPM once volume is known.

## Monthly blended example (about 10,000 registered users)

Assumptions, for illustration only: 72 listings a month, 2 % of users on a paid plan, two
local ad campaigns, defaults as above.

| Line | Volume | Athlix net |
|---|---|---|
| Listings, typical (case A) | 40 × ₹388.86 | ₹15,554 |
| Listings, small at minimum (case B) | 20 × ₹96.24 | ₹1,925 |
| Listings, free events (case D) | 10 × ₹96.24 | ₹962 |
| Listings, large at cap (case C) | 2 × ₹4,859.79 | ₹9,720 |
| **Listing fees** | 72 orders, ₹34,182 charged incl. ₹5,214 GST | **₹28,161** |
| Player Plus | 150 × ₹71.31 | ₹10,697 |
| Coach Pro | 35 × ₹215.38 | ₹7,538 |
| Organiser Pro | 15 × ₹359.45 | ₹5,392 |
| **Premium** | 200 subscribers | **₹23,627** |
| **Ads** | 2 campaigns × ₹5,000 (ex-GST) | **₹10,000** |
| **Total net revenue** | | **≈ ₹61,800 / month** |

Running costs at that scale are roughly ₹25,000 a month (AWS, DigiLocker verification, SMS,
store and domain fees; [costs.md](costs.md)), so the platform covers its infrastructure. It
does not cover salaries, marketing, legal or accounting; those are the real cost of the
business and are not in this table.

Sensitivities worth tracking from day one (all visible in the admin dashboard):

- **Listings per month** and the share hitting the minimum: if most events are small, the
  ₹99 minimum dominates; consider a lower % with a higher minimum, or per-sport rules.
- **Refund rate** (cancellations before registration and admin rejections at 100 %).
- **Premium conversion** (store fees take 15 %; churn is monthly).
- **Verification cost per new user** (DigiLocker via an aggregator is a per-call cost paid
  before the user ever pays anything).

Author: Atindraa.
