// Package secrets reads secret values from AWS Secrets Manager (cached in memory only).
//
// Locally (ENV=local) it reads backend/.secrets.local.json instead, a gitignored file created
// by `make dev-secrets` with random development keys. Never put real keys in that file.
package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// Getter fetches a secret by full name and decodes its JSON into out.
type Getter interface {
	GetJSON(ctx context.Context, name string, out any) error
}

// AWS is a Secrets Manager backed Getter with an in-memory cache for the process lifetime.
type AWS struct {
	client *secretsmanager.Client
	mu     sync.Mutex
	cache  map[string][]byte
}

// NewAWS builds a Getter from an AWS config (uses the Lambda/ECS IAM role).
func NewAWS(cfg aws.Config) *AWS {
	return &AWS{client: secretsmanager.NewFromConfig(cfg), cache: map[string][]byte{}}
}

func (a *AWS) GetJSON(ctx context.Context, name string, out any) error {
	a.mu.Lock()
	raw, ok := a.cache[name]
	a.mu.Unlock()
	if !ok {
		res, err := a.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(name)})
		if err != nil {
			return fmt.Errorf("secrets: get %s failed", name) // never wrap the SDK error body
		}
		if res.SecretString == nil {
			return fmt.Errorf("secrets: %s has no string value", name)
		}
		raw = []byte(*res.SecretString)
		a.mu.Lock()
		a.cache[name] = raw
		a.mu.Unlock()
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("secrets: %s is not valid JSON for the expected shape", name)
	}
	return nil
}

// LocalFile reads secrets from a JSON object file: {"athlix/local/jwt": {...}, ...}.
type LocalFile struct {
	data map[string]json.RawMessage
}

// NewLocalFile loads path. Used only when ENV=local.
func NewLocalFile(path string) (*LocalFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("secrets: cannot read %s (run `make dev-secrets`)", path)
	}
	l := &LocalFile{}
	if err := json.Unmarshal(b, &l.data); err != nil {
		return nil, fmt.Errorf("secrets: %s is not a JSON object", path)
	}
	return l, nil
}

func (l *LocalFile) GetJSON(_ context.Context, name string, out any) error {
	raw, ok := l.data[name]
	if !ok {
		return fmt.Errorf("secrets: %s missing from local secrets file", name)
	}
	return json.Unmarshal(raw, out)
}

// Static is an in-memory Getter for tests.
type Static map[string]any

func (s Static) GetJSON(_ context.Context, name string, out any) error {
	v, ok := s[name]
	if !ok {
		return fmt.Errorf("secrets: %s missing", name)
	}
	b, _ := json.Marshal(v)
	return json.Unmarshal(b, out)
}
