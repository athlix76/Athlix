# cmd/admincli/testdata

Fixture files for `main_test.go`. Go tooling ignores this folder when building.

- `districts.csv`: a three-line LGD CSV (header, one valid Tamil Nadu row, one row with an
  unknown state code) used by the `import-districts` test.

To change safely: keep the files tiny and fictional, never paste production data or real
personal data here, and update the sqlmock expectations in `main_test.go` to match.
