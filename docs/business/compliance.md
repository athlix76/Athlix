# Compliance

> **This is not legal, tax or regulatory advice.** It is a working map, written by the product
> team, of the Indian laws and platform rules that appear to apply to Athlix and how the
> product is designed around them. Laws, rules and their commencement dates change. **A
> qualified Indian lawyer must review this document, the policies and the product before
> launch**, and a chartered accountant must review the GST position. Record who reviewed which
> section and when at the bottom of this page.

## Summary

| Area | Why it applies | Product position | Ask the lawyer / CA |
|---|---|---|---|
| DPDP Act 2023 + DPDP Rules 2025 | Athlix processes personal data of Indian users, including children | Consent, minimal data, encrypted PII, deletion, grievance officer, parental consent | Is OTP to the parent's mobile "verifiable" consent? Notice wording, retention, breach process |
| IT Act + IT Rules 2021 | Athlix hosts user content (profiles, posts, chats, events) | Published rules, grievance officer, report and takedown tools, logs retained | Timelines, takedown process, law enforcement requests |
| Aadhaar Act / UIDAI | DigiLocker returns Aadhaar-derived data | Keep last 4 digits only; never number, XML, PDF or photo | Is DigiLocker issued-document access the right legal basis? |
| DigiLocker partner onboarding | Needed for direct DigiLocker API access | Direct or via an approved aggregator (`verification.provider`) | Which route, contracts |
| RBI payment aggregator rules | Money flows between players and organisers | Athlix never holds entry fees; it is a Razorpay **merchant** for its own fee | Confirm Athlix is not a PA; review event text and organiser terms |
| GST | Athlix sells listing fees, plans and ads | 18 % GST on listing fees (`gstBp`) | Registration, SAC codes, e-commerce operator status, invoices |
| App store payment rules | Premium plans and the listing fee are paid in the app | Plans via store billing; listing fee via Razorpay | Is the listing fee "digital content" for Apple / Google? |
| Consumer Protection (E-Commerce) Rules 2020 | Athlix is an online platform where organisers offer events | Organiser details, price breakdown, refund policy, grievance handling | Is Athlix a "marketplace e-commerce entity"? Dark patterns guidelines |

## DPDP Act 2023 (Digital Personal Data Protection)

Athlix is a **Data Fiduciary** for every user. The Digital Personal Data Protection Rules,
2025 were notified in November 2025 with a phased commencement (most obligations a little
over a year after notification); **confirm which provisions are in force on the launch date**.
Building to the full Act from day one is cheaper than retrofitting it.

### Consent and notice

- Consent must be free, specific, informed, unconditional and unambiguous, for a stated purpose,
  given after a clear notice in English or a scheduled Indian language. The privacy policy is
  `policy_documents` type `privacy` (`requires_acceptance = 1`); a new version blocks the app
  until accepted (`POST /me/policies/accept`), and acceptances are recorded per version in
  `policy_acceptances`.
- Collect only what each purpose needs. Athlix asks for mobile (login), the DigiLocker result
  (identity and age), sport profile data (the product), and nothing else.
- Users can withdraw consent and delete the account (`DELETE /me`): PII ciphertext is wiped,
  tokens revoked, content hidden. The DigiLocker subject hash is kept 180 days for fraud
  prevention, then cleared ([security.md](../security.md#pii-inventory)); the lawyer must
  confirm that retention and the notice text that discloses it.
- Data principal rights: access, correction, erasure, grievance redressal, and nominating
  someone to act on their behalf. Today access / correction are self-service in the profile
  screens; requests beyond that go to the grievance officer. Document the response time.

### Children's data

- Under DPDP a **child is anyone under 18**. Athlix's minor threshold is
  `guardian.minor_age_years` (default 18). **Do not lower it below 18** while DPDP applies.
- Processing a child's data needs **verifiable consent of a parent or lawful guardian**. Athlix
  sends an OTP to the guardian's own mobile (must differ from the child's), records the
  guardian's name (encrypted), relation, the consent text version and the time.
  **Open question for the lawyer:** the DPDP Rules expect the fiduciary to check that the person
  consenting is an identifiable adult (for example through details already held or a
  DigiLocker-style token). An OTP proves control of a mobile number, not adulthood. Options:
  ask the guardian to verify with DigiLocker too, or accept the risk with a documented
  rationale.
- **No tracking, behavioural monitoring or targeted advertising directed at children.** Athlix
  shows minors no ads (`ads.show_to_minors = false`, keep it so), records no profile views by
  minors, and uses no third-party analytics or ad SDKs.
- Minors get safer defaults (followers-only profile, limited DMs, limited search fields).
- Users below `auth.min_age_years` (default 10) are refused.

### Security, breach notice and retention

- Reasonable security safeguards: encryption of PII at rest (AES-256-GCM), hashing for lookup,
  secrets only in AWS Secrets Manager, data in AWS Mumbai (ap-south-1), least-privilege IAM,
  audited admin access ([security.md](../security.md)).
- **Personal data breach:** notify the Data Protection Board of India **and every affected
  user** without delay; the Rules require a detailed report to the Board within 72 hours of
  becoming aware (confirm). Write the incident runbook before launch: who decides, who drafts,
  where the user notice goes (in-app notification + SMS), and keep a breach register.
- Erase data when the purpose is served or consent is withdrawn, unless a law requires keeping
  it (for example GST records, IT Rules log retention). Write a retention schedule per table.
- Processors (AWS, Razorpay, MSG91, Firebase, the DigiLocker aggregator) need contracts with
  data processing terms.

### Location identities (tournament eligibility)

Players prove where they are **from** (native) or where they **live** (residence) so organisers
can run district or state tournaments ([flows/location-eligibility.md](../flows/location-eligibility.md)).

- **Purpose limitation:** location identities are used only to decide eligibility for events
  and to show "Eligible via ..." tags. They are not used for ads, recommendations or search
  ranking, and organisers see only the qualifying label (for example "Native: Thanjavur
  (verified)"), never the document.
- **Data minimisation:** only the **district** (and its state), the proof type, the status and
  the dates are kept. No address is stored, also not from DigiLocker. For a masked Aadhaar the
  reviewer may record the **last 4 digits** at most. A document that shows a full Aadhaar number
  is rejected (`unmasked_aadhaar`) and the file is deleted at once.
- **Storage limitation:** the proof file is deleted `location.proof_retention_days` (seed
  default 30) after review by the `location.purge_proofs` job, at once on withdrawal, and on
  account deletion. Until then it sits in the private media bucket and reviewers see it only
  through a short signed URL; every view is written to the audit log.
- **Children:** a minor may submit a **parent or guardian's** address proof for residence
  (`location.minor_guardian_proof_allowed`). That proof is the guardian's personal data too; the
  notice must say so and the guardian consent already recorded must cover it (lawyer to confirm).
- **Notice:** the user accepts a separate `location_identity_notice` policy before submitting;
  the version is recorded on the row. **The notice text must be reviewed by a lawyer before
  launch**, including the retention period and the guardian case.
- Native identity from DigiLocker reuses the existing consented DigiLocker share; the lawyer
  should confirm that deriving the district from the Aadhaar address is covered by that
  consent and notice.

### Grievance officer and contact

- Publish the name and contact of a person who answers data protection questions. The app
  shows `app.grievance_officer` (seed: "To be appointed"). **Set it before launch.**
- Significant Data Fiduciary duties (DPO, audits, DPIA) apply only if the government notifies
  Athlix as one; not expected at 1 lakh users, but children's data is a factor the government
  can weigh.

## IT Act 2000 and IT Rules 2021 (intermediary duties)

Athlix hosts content created by users (profiles, posts, comments, messages, community groups,
event listings), so it is an **intermediary** and keeps safe-harbour protection only if it
follows the due-diligence duties of the Information Technology (Intermediary Guidelines and
Digital Media Ethics Code) Rules, 2021 (as amended):

- Publish rules and regulations, a privacy policy and a user agreement in the app, and tell
  users what content is prohibited. Athlix: `terms`, `privacy`, `community_guidelines`
  policy documents, accepted at sign-up and on every new version, plus a periodic reminder.
- **Grievance officer** (resident in India) published in the app: acknowledge complaints
  within **24 hours** and resolve within **15 days**; certain content (for example
  non-consensual intimate imagery) must be removed within **24 hours** of a complaint.
- Remove or disable unlawful content within **36 hours** of a court order or a government
  notice (confirm current timelines; amendments have shortened some).
- On a lawful request, provide information to authorised agencies within **72 hours**.
- Keep information about removed content and registration data of users for **180 days**
  (or longer if required for an investigation). Align this with the DPDP retention schedule.
- Users can appeal a grievance officer decision to the Grievance Appellate Committee.
- Athlix is far below the 50 lakh user threshold for "significant social media intermediary"
  duties (chief compliance officer, monthly reports, first-originator tracing).

Product support: report from any screen (`POST /reports`), auto-hide at
`moderation.auto_hide_report_threshold`, admin queue with `underage_safety` first, actions
recorded in `moderation_actions` and `admin_audit_log` ([flows/moderation.md](../flows/moderation.md)).

## Aadhaar Act 2016 and UIDAI limits

- After the Supreme Court's 2018 Puttaswamy judgement, a **private company cannot use Aadhaar
  authentication** (biometric or OTP e-KYC through UIDAI) unless a law or a specific
  government approval allows it. Later amendments allow voluntary offline verification and,
  for some purposes, approved private entities; Athlix does **not** do Aadhaar authentication.
- Athlix gets identity through **DigiLocker**, with the user's consent, as the user shares a
  document from their own DigiLocker account. Only the fields needed are read: name, date of
  birth, gender and the last 4 digits.
- The Aadhaar Act forbids anyone who obtains an Aadhaar number from publishing, displaying or
  storing it beyond what the law permits, and requires masking. Hence the product rules:
  - **Stored:** last 4 digits only (`identity_verifications.aadhaar_last4`), shown as
    `XXXX XXXX 1234`. This is the same as UIDAI's masked Aadhaar.
  - **Never stored, logged or returned:** the full 12-digit number, the e-Aadhaar XML, the
    PDF, the photo, the DigiLocker access token.
  - Name and DOB are encrypted; only an audited admin "reveal" can show the name.
  - One identity = one account via a keyed hash of the provider subject id, not the number.
- Aadhaar must stay voluntary in spirit: the lawyer should confirm whether another DigiLocker
  document (for example a driving licence) must be accepted as an alternative for adults.

## DigiLocker partner onboarding

- Direct DigiLocker API access ("Requester" / partner) needs an **onboarding application to
  DigiLocker (MeitY)** by a registered organisation, with organisation documents, a described
  use case, technical integration, and approval before production keys are issued.
  **It cannot be done before the company is registered.**
- Alternative: an **approved verification aggregator** that already has DigiLocker access
  (`verification.provider = aggregator`, secret `kyc-aggregator`). Faster to start; costs per
  verification; needs a data processing agreement.
- Until one is live, the `mock` provider is used in dev and is refused in production.
- Keep the consent screen text, purpose and data list identical between the DigiLocker
  application, the privacy policy and the app.

## RBI payment aggregator rules

- RBI regulates **payment aggregators (PAs)**: entities that receive payments from customers
  on behalf of merchants, pool them and settle to the merchants later. A non-bank PA needs RBI
  authorisation, a substantial minimum net worth, escrow account controls, merchant KYC and
  settlement timelines (RBI's 2020 PA / PG guidelines and later directions). Marketplaces that
  collect for their sellers must either be authorised or route the money through an authorised
  PA's marketplace product.
- The concept document's design (players pay Athlix, Athlix holds and releases 50 % + 50 % to
  organisers) would have made Athlix an aggregator or required an escrow / route arrangement.
- **Athlix's design avoids this:** entry fees go **directly** from player to organiser
  (cash, UPI, bank transfer, the organiser's own payment link). Athlix receives only its **own**
  listing fee, as an ordinary **merchant of Razorpay** (Razorpay is the licensed aggregator).
  Athlix does not take a cut of entry fees, does not refund them, and does not show a
  "payment protected by Athlix" message.
- The app must say so clearly: the `entry_fee_disclaimer` and `organiser_terms` policy texts.
- Razorpay merchant onboarding needs the company's KYC, a website with terms, privacy, refund
  and contact pages, and a description of the business that matches this model.
- **Ask the lawyer:** confirm the merchant-only position, and review the "online link" payment
  mode (it must be the organiser's own link, never an Athlix account).

## GST

- Athlix supplies its own services: event listing / promotion, subscriptions, advertising.
  These are taxable at **18 %** (confirm the SAC code for each with the CA).
- Registration is mandatory once aggregate turnover passes the threshold (₹20 lakh for
  services in most states), but **you cannot charge GST without a GSTIN**. Either register
  before launch (recommended, also needed for input tax credit on AWS, Razorpay and store
  fees) or set `gstBp = 0` on the fee rules until registered.
- Athlix does **not** collect consideration for the organisers' supplies (entry fees), so it
  should not be an e-commerce operator required to collect TCS under section 52 on entry fees.
  **Confirm with the CA.**
- Invoices: each paid listing order needs a GST invoice to the organiser (with the
  organiser's GSTIN if they have one, for B2B input credit). The current data model has no
  invoice number or organiser GSTIN; add them before charging GST.
- In-app purchases: check how Google Play and Apple handle Indian GST on subscription sales
  for the developer's account and whether Athlix must remit it.
- Ads are invoiced directly with GST.

## App store payment rules

- **Digital goods and subscriptions** used inside the app must use **Google Play Billing** and
  **Apple In-App Purchase**. Premium plans follow this (`POST /premium/purchases/verify`).
  Google Play also offers user-choice / alternative billing in India at a reduced fee; Athlix
  does not use it at launch.
- **Physical services consumed outside the app** (a real tournament, an in-person coaching
  session) may be paid outside store billing. Entry fees and consultations are paid directly
  to the organiser / coach, outside the app entirely, which is allowed.
- **Open risk: the listing fee.** Paying to *list and promote* an event inside the app may be
  treated by Apple or Google as a digital service that must use in-app purchase, even though
  the event itself is physical. Mitigations to discuss before submission: argue it as a
  business-to-business service for a physical event, take listing payments on the web
  (organiser web flow) instead of in the app, or move listing payment to IAP (and lose ~15 %).
  Ask during store review, and do not hide it.
- Store subscription rules: clear price and renewal terms before purchase, easy cancellation
  through the store, restore purchases, no misleading free-trial wording.
- Account deletion must be possible inside the app (both stores require it): `DELETE /me`.

## Consumer protection (E-Commerce) rules

The Consumer Protection Act 2019 and the Consumer Protection (E-Commerce) Rules, 2020 apply to
digital platforms offering goods or services. Organisers offer events to players through
Athlix, so Athlix may be a **marketplace e-commerce entity** even though it does not take the
entry fee. Likely duties:

- Show Athlix's legal name, address, customer care contact and **grievance officer**;
  acknowledge consumer complaints within **48 hours** and resolve within **one month**.
- Show the **seller's (organiser's) details**: name, organisation, contact (Athlix shows
  `contactName`, organiser profile, and the contact via `GET /events/:id/contact`).
- Show the total price and all charges up front (the listing fee quote shows the GST line; the
  event shows the entry fee and payment modes).
- Show refund, cancellation and complaint terms before purchase (the `listing_refund` policy
  and percentages; the `entry_fee_disclaimer` for entry fees).
- No manipulated prices, fake reviews or misleading ranking. "Featured" and "Sponsored" items
  are labelled.
- The Guidelines for Prevention and Regulation of **Dark Patterns**, 2023 (CCPA): no drip
  pricing (GST is in the quote), no false urgency, no subscription traps, no confirm-shaming.

## Other points to raise

- **Child safety:** adult-to-minor contact is restricted by `minor.dm_policy`; the lawyer
  should review the POCSO reporting duty and how `underage_safety` reports are escalated to
  the police.
- **Sports bodies and trademarks:** organisers must not misuse association names or logos.
- **Data residency:** all data in AWS ap-south-1; FCM and stores process minimal data abroad.

## Review log

| Section | Reviewer | Date | Notes |
|---|---|---|---|
| (none yet) | | | |

Author: Atindraa.
