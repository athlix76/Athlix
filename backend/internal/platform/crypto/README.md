# `internal/platform/crypto`

Package crypto encrypts sensitive PII at rest and builds keyed lookup hashes.

  - Encryption: AES-256-GCM, random 12-byte nonce per value, key versioned so
keys can rotate. Ciphertext layout: [2-byte big-endian key version][12-byte
nonce][ciphertext+tag].
  - Lookup: HMAC-SHA256 with a separate secret key, scoped by a purpose string
so the same value hashed for two purposes gives different hashes.

Keys come ONLY from the "pii-keys" secret in AWS Secrets Manager (see
docs/security.md).


## Exported API

```go
func Last4(s string) string
func MaskMobile(last4 string) string
func RandomToken(n int) (string, error)
func SHA256(s string) []byte
type Box struct
func NewBox(k Keys) (*Box, error)
func (b *Box) Decrypt(data []byte) ([]byte, error)
func (b *Box) DecryptString(data []byte) (string, error)
func (b *Box) Encrypt(plain []byte) ([]byte, error)
func (b *Box) EncryptString(s string) ([]byte, error)
func (b *Box) Hash(purpose, value string) []byte
func (b *Box) KeyVersion() uint16
type Keys struct
```

_Generated from the Go doc comments (`go doc -all ./internal/platform/crypto`). Keep the package comment true and update this file when the API changes._
