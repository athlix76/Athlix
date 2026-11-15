# Tournament listing (listing fee)

An organiser creates an event, sees the listing fee, pays it through Razorpay, and (by default)
waits for admin review before the event is published. **Only the listing fee goes through
Athlix.** Entry fees are paid by players directly to the organiser
([registration.md](registration.md)).

Before creating events the organiser needs role `organiser` and, when
`verification_requirements` (`organiser_approval` / `create_event`) says so, an approved
organiser profile (403 `ORGANISER_NOT_APPROVED`).

## Fee

```
base  = entryFeePaise == 0 ? freeEventFeePaise : clamp(round(entryFeePaise × maxSlots × percentBp / 10000), minFeePaise, maxFeePaise)
total = base + round(base × gstBp / 10000)
```

From the most specific active `listing_fee_rules` row. Default: 5 %, min ₹99, max ₹4,999,
₹99 for free events, 18 % GST. ₹500 × 16 = ₹8,000 → ₹400 + ₹72 GST = **₹472**. The quote is
snapshotted on the order.

## Pay and publish

```mermaid
sequenceDiagram
    autonumber
    actor O as Organiser
    participant App as Athlix app
    participant API as Athlix API
    participant RP as Razorpay
    participant W as Worker
    actor A as Admin

    O->>App: Fill in event details
    App->>API: GET /listing/quote?sportId&eventType&entryFeePaise&maxSlots
    API-->>App: ListingQuote, calculator shown while editing
    App->>API: POST /events EventInput
    API-->>App: Event with status draft
    App->>API: POST /events/:id/listing/quote {}
    API-->>App: ListingQuote with refundPolicy and refundPolicyDocumentId
    O->>App: Accept fee and refund policy
    App->>API: POST /events/:id/listing/order {}
    API->>RP: Create order for quote.totalPaise
    API-->>App: ListingOrder {orderId, providerOrderId, providerKeyId, expiresAt}
    Note over API: event status pending_payment
    App->>RP: Open Razorpay Checkout with providerKeyId and providerOrderId
    O->>RP: Pay with UPI, card or netbanking
    RP-->>App: providerPaymentId and providerSignature
    App->>API: POST /events/:id/listing/confirm {orderId, providerPaymentId, providerSignature}
    Note over API: verify HMAC of orderId and paymentId with keySecret
    API-->>App: ListingOrder with status paid
    par Backup path
        RP->>API: POST /webhooks/razorpay payment.captured or order.paid
        Note over API: verify X-Razorpay-Signature first, dedupe on provider event id, no-op if already paid
        API-->>RP: 200
    end
    Note over API: emits listing.paid
    alt event.require_admin_approval is true
        Note over API: event pending_review, emits event.submitted
        W->>O: Notification your event is in review
        A->>API: POST /admin/events/:id/approve {}
        API-->>A: Event with status published
        Note over API: emits event.published
    else approval not required
        Note over API: event published, emits event.published
    end
    W->>W: chat creates the event group if event.auto_create_group
    W->>O: Notification your event is live
```

- A quote with `totalPaise` 0 (for example a 0 % promotional rule) is marked paid at once, with
  provider `none`.
- Only one open (`created`) order per event; a new order request returns it if the quote is
  unchanged, else expires it and creates a new one.
- `confirm` and the webhook are both idempotent; whichever arrives first marks the order paid.
- Webhook payloads are never stored; only `payment_webhook_events(provider, provider_event_id)`
  for dedupe. A bad signature gets 400 / 401 without a reason.
- `POST /listing/mock/pay` replaces Razorpay when `listing.payment_provider` is `mock` (ENV not prod).

## Unpaid order expiry

The job `listing.expire_orders` (every 15 minutes) moves `created` orders past `expiresAt`
(`listing.order_ttl_minutes`, default 30) to `expired` and the event back to `draft`.
`payment.failed` from the webhook marks the order `failed`; the organiser can order again.

## Refunds: reject, cancel, remove

```mermaid
sequenceDiagram
    autonumber
    actor O as Organiser
    actor A as Admin
    participant API as Athlix API
    participant W as Worker
    participant RP as Razorpay

    alt Admin rejects in review
        A->>API: POST /admin/events/:id/reject {reason}
        Note over API: event rejected, emits event.rejected
        W->>RP: Refund listing.refund.on_admin_rejection_percent of the order, default 100
    else Organiser cancels
        O->>API: POST /events/:id/cancel {reason}
        Note over API: event cancelled, emits event.cancelled
        W->>RP: Refund before_first_registration_percent if no registrations, default 100, else after_registrations_percent, default 0
    else Admin removes a published event
        A->>API: POST /admin/events/:id/remove {reason}
        Note over API: event removed, emits event.cancelled
        Note over W: status removed means rule violation, no refund
    end
    RP->>API: POST /webhooks/razorpay refund.processed
    Note over API: order refunded or partially_refunded, emits listing.refunded
    W->>O: Notification about the refund
```

- The refund runs in `listingpayment` as a handler of `event.rejected` / `event.cancelled`,
  idempotent per order; a 0 % refund does nothing.
- Finance can also refund manually: `POST /admin/listing-orders/:id/refund {amountPaise, reason}`
  (permission `payments.refund`).
- `event.cancelled` also archives the event group chat and notifies every registered player.
- A rejected event is editable and can be resubmitted with a new order.

## Event status

```mermaid
stateDiagram-v2
    [*] --> draft: POST /events
    draft --> pending_payment: listing order created
    pending_payment --> draft: order expired
    pending_payment --> pending_review: paid and approval required
    pending_payment --> published: paid and no approval needed
    pending_review --> published: admin approve
    pending_review --> rejected: admin reject
    rejected --> pending_payment: edited and new order
    published --> completed: job event.complete_past after endsAt
    draft --> cancelled: organiser cancel
    pending_payment --> cancelled: organiser cancel
    pending_review --> cancelled: organiser cancel
    published --> cancelled: organiser cancel
    published --> removed: admin remove
    completed --> [*]
    cancelled --> [*]
    removed --> [*]
```

## Listing order status

```mermaid
stateDiagram-v2
    [*] --> created: POST /events/:id/listing/order
    created --> paid: confirm or webhook
    created --> failed: payment.failed
    created --> expired: job listing.expire_orders
    paid --> refunded: full refund processed
    paid --> partially_refunded: partial refund processed
    partially_refunded --> refunded: further refund
    paid --> [*]
    failed --> [*]
    expired --> [*]
    refunded --> [*]
```

## Editing after payment

`pending_review` events cannot be edited (409). Published events may change only
`description, rules, prizeInfo, paymentInstructions, bannerUploadId, contactName,
contactPhone, venueName, addressLine`; the fee inputs (`entryFeePaise`, `maxSlots`) and dates
are locked. Edits emit `event.updated`, which notifies registered players.
