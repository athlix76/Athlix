# Event registration

A player registers for a published event (or joins the waitlist), sees the organiser's
contact and payment instructions, and **pays the organiser directly** (cash, UPI, bank
transfer or the organiser's own link). The organiser marks the registration as paid; Athlix
only records it. The registration screen shows the `entry_fee_disclaimer` policy.

## Register, reveal contact, pay offline

```mermaid
sequenceDiagram
    autonumber
    actor P as Player
    participant App as Athlix app
    participant API as Athlix API
    participant W as Worker
    actor O as Organiser

    App->>API: GET /events/:id
    API-->>App: Event with entryFeePaise, paymentModes, contactPhoneMasked, slotsLeft
    App->>API: GET /policies/entry_fee_disclaimer
    P->>App: Register, team name and members for team events
    App->>API: POST /events/:id/registrations {teamName, teamMembers}
    alt Not published, deadline passed, own event, age or gender not eligible
        API-->>App: 409 INVALID_STATE or 422 VALIDATION_FAILED
    else Already registered
        API-->>App: 409 CONFLICT
    else Full and waitlist off
        API-->>App: 409 LIMIT_REACHED
    else Full and registration.waitlist_enabled
        API-->>App: Registration with status waitlisted
    else Slot available
        API-->>App: Registration with status registered and paymentStatus unpaid
    end
    Note over API: emits registration.created
    W->>W: chat adds a registered player to the event group
    W->>O: Notification new registration

    App->>API: GET /events/:id/contact
    alt event.show_contact allows it and under event.contact_reveals_per_day
        API-->>App: EventContact {contactName, contactPhone, paymentModes, paymentInstructions}
    else Not allowed
        API-->>App: 403 FORBIDDEN
    else Daily limit reached
        API-->>App: 429 RATE_LIMITED
    end
    P->>O: Pays the entry fee directly by cash, UPI, bank or link
    O->>API: PATCH /events/:id/registrations/:regId {paymentStatus paid_offline, paymentNote}
    API-->>O: RegistrationForOrganiser
    Note over API: emits registration.payment_marked
    W->>P: Notification payment marked by the organiser
```

- `event.show_contact`: `always`, or `after_registration` (default: the caller needs a
  `registered` or `waitlisted` registration). The owner can always see it. Reveals are limited
  per user per day by `event.contact_reveals_per_day` (anti-scraping).
- `paymentStatus` values: `unpaid`, `paid_online` (paid on the organiser's own link),
  `paid_offline`, `waived`. Athlix never verifies or holds the money.
- The organiser can reject a registration: `PATCH ... {status: "rejected"}`.
- Organiser lists: `GET /events/:id/registrations?status` (owner only, else 404).
- Player list: `GET /me/registrations?status`.
- Eligibility: age from the verified birth year against `ageMin` / `ageMax` (unknown is
  allowed); `genderCategory` male / female against the verified gender. Max 30 team members.
- Registered players get a reminder `notification.event_reminder_hours` before the start
  (job `notification.event_reminders`).

## Withdraw and waitlist promotion

```mermaid
sequenceDiagram
    autonumber
    actor P as Player
    participant API as Athlix API
    participant W as Worker
    actor Q as First waitlisted player
    actor O as Organiser

    P->>API: DELETE /events/:id/registrations/me
    alt Less than registration.withdraw_until_hours before start
        API-->>P: 409 INVALID_STATE
    else Allowed
        Note over API: registration withdrawn, registrations_count decremented
        API-->>P: 204
        Note over API: emits registration.withdrawn
        W->>W: chat removes the player from the event group
        W->>O: Notification a player withdrew
        opt A player is waitlisted
            Note over API: first waitlisted becomes registered, emits registration.promoted
            W->>W: chat adds the promoted player to the group
            W->>Q: Notification you got a slot
        end
    end
```

Any entry fee already paid is settled between player and organiser; Athlix does not refund
entry fees.

## Registration status

```mermaid
stateDiagram-v2
    [*] --> registered: slot available
    [*] --> waitlisted: full and waitlist enabled
    waitlisted --> registered: promoted after a withdrawal
    registered --> withdrawn: player withdraws
    waitlisted --> withdrawn: player withdraws
    registered --> rejected: organiser rejects
    waitlisted --> rejected: organiser rejects
    withdrawn --> [*]
    rejected --> [*]
```

If the event is cancelled or removed, every registered player is notified through
`event.cancelled` ([tournament-listing.md](tournament-listing.md)).
