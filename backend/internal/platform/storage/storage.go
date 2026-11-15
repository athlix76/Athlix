// Package storage abstracts the private media bucket.
//
// Clients never choose object keys. internal/platform/media builds every key from trusted
// values and stores it in media_uploads; clients only see upload ids and short-lived URLs.
package storage

import (
	"context"
	"errors"
	"time"
)

// Object is the result of Head.
type Object struct {
	Size        int64
	ContentType string
}

// ErrNotFound is returned by Head when the object does not exist.
var ErrNotFound = errors.New("storage: object not found")

// Storage is implemented by S3 (AWS) and Local (developer machine).
type Storage interface {
	// PresignPut returns a URL the client PUTs the file to, plus headers it MUST send.
	PresignPut(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (url string, headers map[string]string, err error)
	// PresignGet returns a short-lived view URL.
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	// Head returns size and content type, or ErrNotFound.
	Head(ctx context.Context, key string) (Object, error)
	// Delete removes the object (idempotent).
	Delete(ctx context.Context, key string) error
}
