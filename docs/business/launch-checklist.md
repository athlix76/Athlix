# Launch checklist

The company is **not registered yet**. Most provider accounts (DigiLocker, Razorpay, DLT
SMS, store organisation accounts) need a registered business, so the order below matters.
Tick items in a copy of this file or in the tracker; keep the reasons for any skipped item.

Legal context for each item: [compliance.md](compliance.md). Costs: [costs.md](costs.md).

## Phase 0: the company

- [ ] **Choose the entity** (with a CA / company secretary):

  | Option | Good for | Watch out |
  |---|---|---|
  | **Private limited company** (SPICe+ on the MCA portal) | Recommended: separate legal entity, limited liability, investors, accepted by DigiLocker, Razorpay, stores | Annual ROC filings and audit, at least 2 directors |
  | One Person Company (OPC) | A single founder | Converts later; some partners prefer Pvt Ltd |
  | LLP | Partners, lighter compliance | Harder to raise equity |
  | Sole proprietorship | Fastest | No separate entity, unlimited liability, weak for DigiLocker / store organisation accounts. Not recommended |

- [ ] Name approval (check the MCA name search **and** the trademark registry for "Athlix").
- [ ] Incorporation; **PAN and TAN** are issued with SPICe+.
- [ ] Optional: DPIIT **Startup India** recognition (tax and IP fee benefits).
- [ ] **GST registration** (needed to charge the 18 % GST in the fee rules; until then set
      `gstBp = 0`). Decide SAC codes with the CA. Plan GST invoices for listing orders (the
      data model needs an invoice number and organiser GSTIN first).
- [ ] **Current bank account** in the company's name (Razorpay settlements, store payouts).
- [ ] State registrations as applicable (Shops and Establishments, professional tax).
- [ ] **Trademark** application for "Athlix" (word mark, then logo), classes 9 (app), 35
      (advertising, online marketplace), 41 (sports events, coaching), 42 (software as a
      service). File early; search first.
- [ ] **Domain** and DNS on Cloudflare; business email on the domain (`support@`,
      `grievance@`, `security@`).
- [ ] Accounting and a CA for monthly GST / TDS filings.

## Phase 1: provider accounts (after incorporation)

- [ ] **DigiLocker**: apply as a partner / requester (organisation documents, use case,
      consent text) **or** sign with an approved verification aggregator. Store credentials in
      Secrets Manager (`digilocker` or `kyc-aggregator`), set `verification.provider`.
- [ ] **Razorpay**: business account with company KYC, website with terms / privacy / refund /
      contact pages, business description = "listing fees for sports events, paid by organisers".
      Create the webhook to `/v1/webhooks/razorpay` (`payment.captured`, `order.paid`,
      `payment.failed`, `refund.processed`), secret `razorpay`, set
      `listing.payment_provider = razorpay`.
- [ ] **MSG91 + DLT**: register the company as a principal entity on a TRAI DLT portal, get
      the entity id, register the sender id (header) and the two templates (login OTP,
      guardian consent OTP). Secret `msg91`, set `auth.sms_provider = msg91`.
- [ ] **Firebase** project for FCM push; service account in the `fcm` secret.
- [ ] **D-U-N-S number** for the company (needed for store organisation accounts).
- [ ] **Google Play Console** organisation account; create the subscription products to match
      `premium_plans.store_product_id_android`; RTDN Pub/Sub push to `/v1/webhooks/google-play`;
      secret `google-play`.
- [ ] **Apple Developer Program** organisation account; App Store Connect subscriptions to
      match `store_product_id_ios`; Server Notifications v2 to `/v1/webhooks/app-store`;
      secret `app-store`.
- [ ] **AWS** account (production separate from dev), set up per [aws-setup.md](../aws-setup.md);
      Cloudflare Access or an IP allowlist in front of the admin panel.

## Phase 2: policies (lawyer-reviewed)

All are `policy_documents` rows (versioned, edited in the admin Policies screen). The seed
versions are drafts and **must be replaced**.

- [ ] `terms`: Terms of Use (includes the intermediary rules and prohibited content).
- [ ] `privacy`: DPDP-compliant notice: purposes, data list, processors, retention,
      rights, grievance officer, children's data.
- [ ] `guardian_consent`: parental consent text (and the decision on how the guardian is
      verified; see [compliance.md](compliance.md#childrens-data)).
- [ ] `community_guidelines`.
- [ ] `organiser_terms`: organisers collect entry fees directly; Athlix is not a party.
- [ ] `entry_fee_disclaimer`.
- [ ] `listing_refund`: must match the `listing.refund.*` percentages.
- [ ] Appoint the **grievance officer** and set `app.grievance_officer`; set
      `app.support_email`.
- [ ] Incident and **breach response runbook** (Data Protection Board + user notice), retention
      schedule per table, processor agreements (AWS, Razorpay, MSG91, Firebase, aggregator).
- [ ] Store listing texts: data safety form (Play) and privacy nutrition labels (Apple) must
      match the privacy policy.
- [ ] Decide the listing-fee store risk ([compliance.md](compliance.md#app-store-payment-rules)).

## Phase 3: security review

- [ ] Walk the hard rules in [security.md](../security.md) for every module; run the backend
      tests and `go vet`.
- [ ] External penetration test of the API, WebSocket and admin panel (focus: ownership /
      IDOR, minors' DMs, auth refresh, webhooks).
- [ ] Secrets only in Secrets Manager; IAM scoped per Lambda; no secrets in Terraform outputs,
      CI variables or the apps.
- [ ] Admin: every admin has TOTP, roles follow least privilege, the super admin is used
      rarely; admin site behind Cloudflare Access / IP allowlist.
- [ ] Backups: RDS automated backups on, **one restore test done**, PII key (`pii-keys`)
      backup procedure documented.
- [ ] Alarms: 5xx rate, Lambda errors, DLQ depth, RDS CPU / storage, webhook failures.
- [ ] Production config: `verification.provider`, `listing.payment_provider`,
      `auth.sms_provider` are real providers (mock / stub are refused in prod), `ENV=prod`,
      `DEV_FIXED_OTP` unset.

## Phase 4: beta

- [ ] **Closed beta in one city** (for example Pune), 2 to 3 sports, 10 to 20 organisers
      invited and approved by hand, 300 to 1,000 players and coaches.
- [ ] Optional launch promotion: a fee rule with 0 % / ₹0 minimum for the beta period (total 0
      is marked paid at once), then the default rule.
- [ ] Keep `premium.enabled` and `ads.enabled` off.
- [ ] Watch: verification success rate and cost, OTP delivery, listing payment success, review
      time, reports per 1,000 users, `underage_safety` reports, crash rate.
- [ ] Weekly review of reports and the audit log; fix the onboarding drop-off first.
- [ ] Store submissions: internal testing (Play) and TestFlight (Apple), then production.

## Phase 5: public launch

- [ ] Open sign-ups in more cities; marketing to academies and associations.
- [ ] Turn on premium (`premium.enabled`) once there are active profiles and events; decide
      whether Coach Pro is required for consultations (`verification_requirements`).
- [ ] Turn on ads (`ads.enabled`) with direct-sold campaigns; keep `ads.show_to_minors` off.
- [ ] Revisit fee rules per sport after the first three months of data.

Author: Atindraa.
