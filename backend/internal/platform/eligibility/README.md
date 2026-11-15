# `internal/platform/eligibility`

Package eligibility decides whether a user may join an event based on their location
identities: **native** (where they are from: DigiLocker Aadhaar district or a reviewed proof)
and **residence** (where they live now: an uploaded proof, renewed every
`location.residence_validity_months`). It is the **only** implementation of the rule. The
event feed, event detail, registration, waitlist promotion, organiser contact, search and the
registration re-check on identity changes all call it. Never re-implement the rule in a module.

Contract: [docs/api.md#location-identity](../../../../docs/api.md),
[docs/architecture.md#location-eligibility](../../../../docs/architecture.md).

## The rule

| Event `eligibility_type` | Who qualifies |
|---|---|
| `open` | everyone |
| `native` / `residence` | a user with an identity of that type in the event's district (scope `district`) or any district of its state (scope `state`) |
| `either` | native or residence; native is preferred when both match (it never expires) |

- Only `verified` identities whose `expires_at` has not passed count. A `pending` identity also
  counts when the event has `require_verified = false` (the result then has `Pending: true`).
- Ineligible results carry a closed `Reason` (`identity_missing`, `identity_pending`,
  `identity_expired`, `wrong_location`) and an `Action` the app shows
  (`verify_native`, `verify_residence`, `renew_residence`, `wait_for_review`).
- `location.enabled = false` makes every event open (feature switch), with no DB call.

## Exported API

```go
type Rule struct{ Type, Scope string; DistrictID, StateID int64; RequireVerified bool }
func (r Rule) Validate() map[string]string           // field errors: eligibility.type|scope|districtId|stateId
type Identity struct{ ID int64; Type string; DistrictID, StateID int64; Status string; ExpiresAt *time.Time }
type Result struct{ Eligible bool; Via string; IdentityID, DistrictID int64; Pending bool; Reason, Action string }

func Evaluate(r Rule, ids []Identity, now time.Time) Result         // pure: use for many events after one Load
func Filter(alias string, ids []Identity, now time.Time) (string, []any) // SQL condition + bind args for `events <alias>`
func Describe(r Rule, place string) string  // "Only players with a verified Thanjavur native identity can register."
func Tag(res Result, place string) string   // "Eligible via Native: Thanjavur"

type Checker struct{ ... }                  // app.Deps.Eligibility
func New(d *sql.DB, st *settings.Store) *Checker
func NewWithPlaces(d *sql.DB, st *settings.Store, p *Places) *Checker
func (c *Checker) Enabled(ctx) bool
func (c *Checker) Load(ctx, q db.Querier, userID int64) ([]Identity, error) // q = tx inside a transaction, nil = pool
func (c *Checker) Check(ctx, q db.Querier, userID int64, r Rule, now time.Time) (Result, error)
func (c *Checker) FeedFilter(ctx, alias string, userID int64, now time.Time) (string, []any, error) // "1=1" when off
func (c *Checker) PlaceFor(ctx, r Rule) string
func (c *Checker) Places() *Places

type Places struct{ ... }                   // cached states + districts (TTL 10 min)
func (p *Places) State(ctx, id int64) (State, bool)
func (p *Places) District(ctx, id int64) (District, bool)
func (p *Places) StateName(ctx, id int64) string
func (p *Places) DistrictName(ctx, id int64) string
func (p *Places) MatchDistrict(ctx, stateName, districtName string) (District, bool) // name or alias, case/punctuation-insensitive
func (p *Places) Invalidate()                                   // this process only
func (p *Places) Bump(ctx) error                                // after a district edit: Invalidate + INCR places:version
func (p *Places) UseVersionStore(vs VersionStore, every time.Duration) // attach Redis once at startup (0 = 15 s)
const PlacesVersionKey = "places:version"

func TestPlaces() *Places // static fixture: TestStateTN=23, TestStateKL=12, TestDistrictThanj=26, TestDistrictCoimb=4, TestDistrictErnaku=9001
```

## Data

Reads only (owned by `modules/locationidentity`): `user_location_identities`, `states`,
`districts`. It never stores an address or Aadhaar number: `MatchDistrict` maps the DigiLocker
address district to a district id and the address is dropped.

## Changing it safely

- Any change to the rule changes it everywhere at once; that is the point. Update docs/api.md
  and the table above in the same change.
- `Evaluate` and `Filter` must agree. `TestEvaluate` covers the spec cases (eligible native,
  eligible residence, ineligible, expired proof, pending proof, either, state scope,
  `requireVerified=false`, switch off); add a `TestFilter` case for every new `Evaluate` case.
- `Filter` builds SQL from constants plus bind args only; `alias` must be a code-provided
  identifier (anything else panics).
- Cache invalidation across Lambda instances: admin district edits call `Places.Bump` after
  the commit, which drops the local cache and `INCR`s the Redis key `places:version`. Every
  instance with a version store attached (`UseVersionStore`, wired at startup) reads that key
  at most every 15 s (one reader per interval, 250 ms timeout) and reloads when it changed, so
  an edit is visible everywhere within about 15 s. The 10-minute TTL stays as the fallback: a
  Redis error never fails a request or a reload, it only means instances wait for the TTL.
  `Places.Invalidate` alone is per process. `NewStaticPlaces` / `TestPlaces` never reload.
- Real-MySQL check of the SQL: `ATHLIX_TEST_DSN=... go test -tags integration ./internal/platform/eligibility/`
  (writes only rows with user ids >= 900000000).
