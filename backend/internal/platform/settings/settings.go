// Package settings reads runtime business configuration from the app_config table.
//
// Every limit, switch, fee percentage etc. that the business may want to change WITHOUT a
// deploy is an app_config row (documented in docs/config-system.md). Values are cached in
// process for TTL (default 15s), so an admin change takes effect within ~15 seconds on every
// Lambda. Typed config tables (listing_fee_rules, premium_plans, ...) are read by their
// owning modules directly.
//
// Usage:
//
//	n := d.Settings.Int(ctx, "chat.group_max_members", 500)   // default used if key missing
package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"athlix/backend/internal/platform/logger"
)

// Entry is one app_config row.
type Entry struct {
	Key         string          `json:"key"`
	Value       json.RawMessage `json:"value"`
	ValueType   string          `json:"valueType"`
	Category    string          `json:"category"`
	Description string          `json:"description"`
	IsPublic    bool            `json:"isPublic"`
	Version     int             `json:"version"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// Store is a TTL-cached view of app_config.
type Store struct {
	db     *sql.DB
	ttl    time.Duration
	mu     sync.RWMutex
	values map[string]Entry
	loaded time.Time
	now    func() time.Time
}

// New builds a Store. ttl <= 0 means 15s.
func New(db *sql.DB, ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = 15 * time.Second
	}
	return &Store{db: db, ttl: ttl, now: time.Now}
}

// NewStatic builds a Store pre-filled with values and no DB (for tests). Values are Go
// values marshalled to JSON, e.g. map[string]any{"chat.group_max_members": 3}.
func NewStatic(values map[string]any) *Store {
	s := &Store{ttl: 24 * time.Hour, values: map[string]Entry{}, now: time.Now}
	for k, v := range values {
		b, _ := json.Marshal(v)
		s.values[k] = Entry{Key: k, Value: b}
	}
	s.loaded = time.Now()
	return s
}

// Set overrides a value in memory (tests only; admin writes go through the appconfig module).
func (s *Store) Set(key string, v any) {
	b, _ := json.Marshal(v)
	s.mu.Lock()
	if s.values == nil {
		s.values = map[string]Entry{}
	}
	s.values[key] = Entry{Key: key, Value: b}
	s.mu.Unlock()
}

// Invalidate forces a reload on next read in this process.
func (s *Store) Invalidate() {
	s.mu.Lock()
	if s.db != nil {
		s.loaded = time.Time{}
	}
	s.mu.Unlock()
}

func (s *Store) snapshot(ctx context.Context) map[string]Entry {
	s.mu.RLock()
	fresh := s.values != nil && (s.db == nil || s.now().Sub(s.loaded) < s.ttl)
	vals := s.values
	s.mu.RUnlock()
	if fresh {
		return vals
	}
	loaded, err := s.load(ctx)
	if err != nil {
		logger.From(ctx).Error("settings reload failed; serving previous values", slog.String("err", err.Error()))
		if vals == nil {
			return map[string]Entry{}
		}
		return vals
	}
	s.mu.Lock()
	s.values, s.loaded = loaded, s.now()
	s.mu.Unlock()
	return loaded
}

func (s *Store) load(ctx context.Context) (map[string]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT config_key, value, value_type, category, description, is_public, version, updated_at FROM app_config`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Entry{}
	for rows.Next() {
		var e Entry
		var raw []byte
		if err := rows.Scan(&e.Key, &raw, &e.ValueType, &e.Category, &e.Description, &e.IsPublic, &e.Version, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.Value = raw
		out[e.Key] = e
	}
	return out, rows.Err()
}

func (s *Store) decode(ctx context.Context, key string, out any) bool {
	e, ok := s.snapshot(ctx)[key]
	if !ok {
		logger.From(ctx).Warn("settings key missing; using default", slog.String("key", key))
		return false
	}
	if err := json.Unmarshal(e.Value, out); err != nil {
		logger.From(ctx).Error("settings key has wrong type; using default", slog.String("key", key))
		return false
	}
	return true
}

// Int returns the integer value of key, or def.
func (s *Store) Int(ctx context.Context, key string, def int) int {
	var v int
	if s.decode(ctx, key, &v) {
		return v
	}
	return def
}

// Bool returns the boolean value of key, or def.
func (s *Store) Bool(ctx context.Context, key string, def bool) bool {
	var v bool
	if s.decode(ctx, key, &v) {
		return v
	}
	return def
}

// String returns the string value of key, or def.
func (s *Store) String(ctx context.Context, key, def string) string {
	var v string
	if s.decode(ctx, key, &v) {
		return v
	}
	return def
}

// StringList returns the []string value of key, or def.
func (s *Store) StringList(ctx context.Context, key string, def []string) []string {
	var v []string
	if s.decode(ctx, key, &v) {
		return v
	}
	return def
}

// JSON decodes the value of key into out; returns false (out untouched) if missing.
func (s *Store) JSON(ctx context.Context, key string, out any) bool { return s.decode(ctx, key, out) }

// Duration returns key (an int number of seconds) as a time.Duration, or def.
func (s *Store) Seconds(ctx context.Context, key string, def int) time.Duration {
	return time.Duration(s.Int(ctx, key, def)) * time.Second
}

// Public returns every is_public=1 value, for GET /v1/app/bootstrap.
func (s *Store) Public(ctx context.Context) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for k, e := range s.snapshot(ctx) {
		if e.IsPublic {
			out[k] = e.Value
		}
	}
	return out
}

// All returns every entry (admin config screen).
func (s *Store) All(ctx context.Context) []Entry {
	snap := s.snapshot(ctx)
	out := make([]Entry, 0, len(snap))
	for _, e := range snap {
		out = append(out, e)
	}
	return out
}
