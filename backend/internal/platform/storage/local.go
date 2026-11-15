package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Local emulates presigned URLs on a developer machine. Files live under Dir and are served
// by the /dev/storage routes registered with RegisterRoutes (ENV=local only).
type Local struct {
	Dir     string
	BaseURL string // e.g. http://localhost:8080
	key     []byte
}

// NewLocal builds Local storage with a random per-process signing key.
func NewLocal(dir, baseURL string, signingKey []byte) *Local {
	return &Local{Dir: dir, BaseURL: strings.TrimRight(baseURL, "/"), key: signingKey}
}

func (l *Local) sign(method, key, ct string, exp int64) string {
	m := hmac.New(sha256.New, l.key)
	m.Write([]byte(method + "\n" + key + "\n" + ct + "\n" + strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(m.Sum(nil))
}

func (l *Local) url(method, key, ct string, ttl time.Duration) string {
	exp := time.Now().Add(ttl).Unix()
	v := url.Values{"key": {key}, "exp": {strconv.FormatInt(exp, 10)}, "sig": {l.sign(method, key, ct, exp)}}
	if ct != "" {
		v.Set("ct", ct)
	}
	return l.BaseURL + "/dev/storage/object?" + v.Encode()
}

func (l *Local) PresignPut(_ context.Context, key, ct string, size int64, ttl time.Duration) (string, map[string]string, error) {
	return l.url(http.MethodPut, key, ct, ttl), map[string]string{"Content-Type": ct, "Content-Length": strconv.FormatInt(size, 10)}, nil
}

func (l *Local) PresignGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	return l.url(http.MethodGet, key, "", ttl), nil
}

func (l *Local) path(key string) (string, error) {
	clean := filepath.Clean("/" + key)
	if strings.Contains(key, "..") {
		return "", errors.New("storage: bad key")
	}
	return filepath.Join(l.Dir, clean), nil
}

func (l *Local) Head(_ context.Context, key string) (Object, error) {
	p, err := l.path(key)
	if err != nil {
		return Object{}, err
	}
	st, err := os.Stat(p)
	if err != nil {
		return Object{}, ErrNotFound
	}
	ct, _ := os.ReadFile(p + ".ct")
	return Object{Size: st.Size(), ContentType: string(ct)}, nil
}

func (l *Local) Delete(_ context.Context, key string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	_ = os.Remove(p + ".ct")
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// RegisterRoutes mounts GET/PUT /dev/storage/object (signature-checked, ENV=local only).
func (l *Local) RegisterRoutes(r *gin.Engine) {
	h := func(c *gin.Context) {
		key, ct := c.Query("key"), c.Query("ct")
		exp, _ := strconv.ParseInt(c.Query("exp"), 10, 64)
		if exp < time.Now().Unix() || !hmac.Equal([]byte(c.Query("sig")), []byte(l.sign(c.Request.Method, key, ct, exp))) {
			c.Status(http.StatusForbidden)
			return
		}
		p, err := l.path(key)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		if c.Request.Method == http.MethodGet {
			c.File(p)
			return
		}
		if c.GetHeader("Content-Type") != ct {
			c.Status(http.StatusBadRequest)
			return
		}
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		f, err := os.Create(p)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		defer f.Close()
		if _, err := io.Copy(f, io.LimitReader(c.Request.Body, 50<<20)); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		_ = os.WriteFile(p+".ct", []byte(ct), 0o644)
		c.Status(http.StatusOK)
	}
	r.GET("/dev/storage/object", h)
	r.PUT("/dev/storage/object", h)
}
