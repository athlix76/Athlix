# `internal/platform/secrets`

Package secrets reads secret values from AWS Secrets Manager (cached in memory
only).

Locally (ENV=local) it reads backend/.secrets.local.json instead, a gitignored
file created by `make dev-secrets` with random development keys. Never put real
keys in that file.


## Exported API

```go
type AWS struct
func NewAWS(cfg aws.Config) *AWS
func (a *AWS) GetJSON(ctx context.Context, name string, out any) error
type Getter interface
type LocalFile struct
func NewLocalFile(path string) (*LocalFile, error)
func (l *LocalFile) GetJSON(_ context.Context, name string, out any) error
type Static map[string]any
func (s Static) GetJSON(_ context.Context, name string, out any) error
```

_Generated from the Go doc comments (`go doc -all ./internal/platform/secrets`). Keep the package comment true and update this file when the API changes._
