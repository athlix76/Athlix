# usertest

sqlmock helpers for the user module's shared queries, for tests in auth, verification,
guardian and profile. Test-only code: never import it from production code.

- `ExpectMe(mock, MeRow{...})`: the queries of `user.LoadMe` (user row, roles, pending
  policies, photo view URL when `PhotoID` is set).
- `ExpectRoles(mock, userID, roles...)`: the ordered `user_roles` query.
- `ExpectAdvance(mock, userID, AdvanceRow{...}, next)`: the queries of `user.Advance`,
  including the `onboarding_step` UPDATE when the step changes.

To change safely: when a query in `user/core.go` changes, update the matching regex and
columns here in the same change, then run the tests of every module that uses this package.
