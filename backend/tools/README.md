# backend/tools

`deps.go` (build tag `tools`) imports every third-party package the codebase is allowed to use,
so `go mod tidy` keeps them in `go.mod`. To add a dependency: add it here, then
`go get <module>@<version> && go mod tidy`, and mention why in the change description.
