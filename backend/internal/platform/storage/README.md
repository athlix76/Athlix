# `internal/platform/storage`

Package storage abstracts the private media bucket.

Clients never choose object keys. internal/platform/media builds every key from
trusted values and stores it in media_uploads; clients only see upload ids and
short-lived URLs.


## Exported API

```go
type Local struct
func NewLocal(dir, baseURL string, signingKey []byte) *Local
func (l *Local) Delete(_ context.Context, key string) error
func (l *Local) Head(_ context.Context, key string) (Object, error)
func (l *Local) PresignGet(_ context.Context, key string, ttl time.Duration) (string, error)
func (l *Local) PresignPut(_ context.Context, key, ct string, size int64, ttl time.Duration) (string, map[string]string, error)
func (l *Local) RegisterRoutes(r *gin.Engine)
type Object struct
type S3 struct
func NewS3(cfg aws.Config, bucket string) *S3
func (s *S3) Delete(ctx context.Context, key string) error
func (s *S3) Head(ctx context.Context, key string) (Object, error)
func (s *S3) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
func (s *S3) PresignPut(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (string, map[string]string, error)
type Storage interface
```

_Generated from the Go doc comments (`go doc -all ./internal/platform/storage`). Keep the package comment true and update this file when the API changes._
