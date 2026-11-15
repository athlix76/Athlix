# features/reports

Report a user, event, post, comment, message, conversation or community to moderators. A
community is reported as `ReportTarget.community` (not as its conversation).

## Use

```dart
showReportSheet(context, targetType: ReportTarget.event, targetId: event.id);
```

The sheet lists the contract reasons, takes optional details and sends `POST /reports`.
Reporting the same target twice is fine (the server returns the existing report).

## Folders

- `data/`: `ReportTarget`, `ReportReason` (values and labels), `ReportApi`
- `presentation/`: `showReportSheet` (also re-exports `ReportTarget` and `ReportReason`)

## Changing it safely

New reasons or target types must be added to the server contract first, then to
`ReportReason.all` / `ReportReason.labels` or `ReportTarget`. Never put personal details of the
reported user into `details` automatically.
