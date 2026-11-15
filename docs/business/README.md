# docs/business/

Business documents for the founders: what Athlix is, how it earns money, which Indian laws
apply, what it costs to run and what must happen before launch. They are written for people,
not for code. The code contracts live one folder up ([api.md](../api.md),
[config-system.md](../config-system.md), [security.md](../security.md)).

| File | What it covers |
|---|---|
| [product-overview.md](product-overview.md) | Who Athlix is for, the features, the user journeys, what is in the first launch and what comes later |
| [revenue-model.md](revenue-model.md) | Listing fee, premium plans, ads, coach consultations; worked unit economics; why Athlix never holds entry fees |
| [compliance.md](compliance.md) | DPDP Act, IT Rules 2021, Aadhaar / DigiLocker limits, RBI payment aggregator rules, GST, app store payment rules, consumer protection. **Not legal advice.** |
| [launch-checklist.md](launch-checklist.md) | Phase-by-phase list: company registration → provider accounts → lawyer-reviewed policies → security review → beta |
| [costs.md](costs.md) | Rough AWS and third-party running costs for dev, 10k users and 1 lakh users |

## Sources

- The concept document *Athlix: App Features, User Experience, Revenue Model and Tournament
  Payment Flow* (product intent, journeys, revenue streams).
- The locked product decisions recorded in the root [README.md](../../README.md) and the
  repo rules ([AGENTS.md](../../AGENTS.md)), the most important being that **Athlix never holds player money**.
- The contracts in `docs/` for exact numbers (all defaults below are seed values from
  [config-system.md](../config-system.md) and can be changed in the admin panel without a deploy).

## How to change these files safely

- **Numbers come from config.** When you quote a fee, price, limit or percentage, quote the
  config key or table next to it (for example `listing_fee_rules`, `premium_plans`,
  `listing.refund.*`) and say it is a default. If the seed value changes in a migration, update
  the number here in the same change.
- **Do not describe a flow the product does not have.** In particular, never describe Athlix
  collecting, holding, splitting or refunding an entry fee. If the business model changes,
  change [revenue-model.md](revenue-model.md) and [compliance.md](compliance.md) together, and
  get legal advice first (see the payment aggregator section).
- **Legal text is not legal advice.** [compliance.md](compliance.md) is a map of what to ask a
  lawyer and a chartered accountant. Do not remove its disclaimer, and record the date and
  reviewer when a professional has checked a section.
- **Costs are estimates.** Keep the assumptions table in [costs.md](costs.md) next to any figure
  and refresh prices from the AWS Pricing Calculator and vendor price pages before using them
  in a budget.
- Flows and sequence diagrams belong in [../flows/](../flows/README.md), not here.

Author: Atindraa.
