# `internal/app`

Package app wires the platform into one Deps value and defines the contract
every module implements. Modules live in internal/modules/<name> and are listed
in internal/modules/registry.go.


## Exported API

```go
func CollectJobs(modules []Module) map[string]JobFunc
func NewRouter(d *Deps, modules []Module) *gin.Engine
func RegisterEventHandlers(d *Deps, modules []Module)
type Deps struct
func Bootstrap(ctx context.Context, cfg *config.Config) (*Deps, func(), error)
type EventSubscriber interface
type JobFunc func(ctx context.Context) error
type JobProvider interface
type Module interface
type Routes struct
```

_Generated from the Go doc comments (`go doc -all ./internal/app`). Keep the package comment true and update this file when the API changes._
