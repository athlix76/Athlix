# docs/flows/

Sequence and state diagrams of the important user journeys, in mermaid (renders on GitHub and
in VS Code with a mermaid extension). Each page has a short explanation, the diagram(s), and
the rules that matter. Endpoint paths, domain event names, config keys, job names and
WebSocket frames are copied **exactly** from the contracts:
[api.md](../api.md), [architecture.md](../architecture.md),
[config-system.md](../config-system.md), [security.md](../security.md).

| File | Journey |
|---|---|
| [signup-digilocker.md](signup-digilocker.md) | Mobile OTP sign-up, role, DigiLocker verification (server-side OAuth2 + PKCE), profile, onboarding steps |
| [minor-consent.md](minor-consent.md) | Minor detected from the verified DOB, guardian OTP consent, minor safety rules |
| [tournament-listing.md](tournament-listing.md) | Event draft, listing fee quote, Razorpay order and payment, webhook, admin review, publish; refunds on cancel / reject / remove; order expiry |
| [registration.md](registration.md) | Register, waitlist, contact reveal, paying the organiser offline, organiser marks paid, withdraw and waitlist promotion |
| [chat.md](chat.md) | WebSocket ticket, connect, send, ack, fan-out to online members, push to offline members, REST fallback |
| [premium-purchase.md](premium-purchase.md) | Plans, store purchase with `storeAccountToken`, server verification, store webhooks, expiry |
| [moderation.md](moderation.md) | Report, auto-hide, admin queue, reading reported messages (audited), resolution, suspension |
| [location-eligibility.md](location-eligibility.md) | Native identity from DigiLocker, proof upload and admin review, residence expiry and renewal, feed filter and `showAll`, `NOT_ELIGIBLE` on register, flag-never-cancel, proof purge |

## Conventions in the diagrams

- `API` is the `api` Lambda (locally `cmd/api`); `WS` is the `ws` Lambda; `Worker` is the
  `worker` Lambda that consumes SQS `athlix-events` and runs scheduled jobs. Locally all three
  are one process and events are dispatched in process.
- Paths are shown without the `/v1` prefix, as in api.md.
- "emits `x`" means a domain event published **after** the transaction commits; handlers are
  idempotent because delivery is at least once.
- Numbers in the text are seed defaults; the config key next to them is what the code reads.

## How to change these files safely

- **The contract wins.** If a diagram and api.md / architecture.md disagree, fix the diagram
  (or change the contract and the code together first). Never document an endpoint or event
  that is not in the contracts.
- When you add or rename an endpoint, event, job or config key, search this folder for the old
  name and update every diagram in the same change.
- Keep mermaid valid: one statement per line, no semicolons inside message text, no
  parentheses or special characters in participant aliases, close every `alt` / `opt` /
  `loop` with `end`. Preview before committing.
- Keep diagrams at the level of modules and endpoints. SQL, column lists and Go function names
  belong in the module READMEs under `backend/internal/modules/`.

Author: Atindraa.
