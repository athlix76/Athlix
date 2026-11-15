# location_identity/application

`eligibility_view.dart` (`EligibilityView.from`, reason text, action label / route, `NotEligibleInfo.fromError`), `location_identity_controller.dart` (geo providers, identities, submit with proof upload, withdraw, `SubmitProblem`).

## Changing it safely

Keep decision logic pure so it can be unit tested in `app/test/`.
