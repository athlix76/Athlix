# reports/data

- `report_api.dart`: `ReportTarget` and `ReportReason` (exact strings from docs/api.md),
  `ReportApi.create` and `reportApiProvider`.

Changing it safely: the strings are the contract; change labels freely, values never.
