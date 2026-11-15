package eligibility

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// PlacesVersionKey is the Redis counter that admin district edits increment (Bump). Every
// instance compares it with the version its cache was loaded at and reloads when it moved.
const PlacesVersionKey = "places:version"

// Technical cache timings (not business numbers).
const (
	// PlacesVersionCheckEvery is how often one instance reads the version counter, at most.
	PlacesVersionCheckEvery = 15 * time.Second
	// placesRedisTimeout bounds one version read / bump; on timeout the TTL still applies.
	placesRedisTimeout = 250 * time.Millisecond
)

// VersionStore is the part of the Redis client Places needs (*redis.Client satisfies it).
type VersionStore interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Incr(ctx context.Context, key string) *redis.IntCmd
}

// UseVersionStore attaches the cross-instance version counter: each instance reads
// PlacesVersionKey at most every `every` (0 = PlacesVersionCheckEvery) and reloads when it
// changed. Call once at startup, before the cache is shared. A nil store keeps TTL-only
// behaviour. A static cache (NewStaticPlaces, TestPlaces) never reloads either way.
func (p *Places) UseVersionStore(vs VersionStore, every time.Duration) {
	if every <= 0 {
		every = PlacesVersionCheckEvery
	}
	p.mu.Lock()
	p.vs, p.every = vs, every
	p.mu.Unlock()
}

// Bump drops this process's cache and increments the shared version so every other instance
// reloads within PlacesVersionCheckEvery. Call it after the district change has committed.
// On a Redis error the other instances still reload within the TTL; the error is returned
// for logging only and must not fail the request.
func (p *Places) Bump(ctx context.Context) error {
	p.Invalidate()
	p.mu.RLock()
	vs := p.vs
	p.mu.RUnlock()
	if vs == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), placesRedisTimeout)
	defer cancel()
	return vs.Incr(ctx, PlacesVersionKey).Err()
}

// checkVersion reads the version if the last read is older than the check interval. Only one
// caller per interval does the read (compare-and-swap on lastCheck); the others get ok=false.
func (p *Places) checkVersion(ctx context.Context) (int64, bool) {
	p.mu.RLock()
	vs, every := p.vs, p.every
	p.mu.RUnlock()
	if vs == nil {
		return 0, false
	}
	now := time.Now().UnixNano()
	last := p.lastCheck.Load()
	if now-last < int64(every) || !p.lastCheck.CompareAndSwap(last, now) {
		return 0, false
	}
	return p.readVersion(ctx)
}

// readVersion returns the shared version (0 when the key does not exist yet). ok=false on
// any Redis error or when no store is attached: the caller then relies on the TTL.
func (p *Places) readVersion(ctx context.Context) (int64, bool) {
	p.mu.RLock()
	vs := p.vs
	p.mu.RUnlock()
	if vs == nil {
		return 0, false
	}
	ctx, cancel := context.WithTimeout(ctx, placesRedisTimeout)
	defer cancel()
	v, err := vs.Get(ctx, PlacesVersionKey).Int64()
	if errors.Is(err, redis.Nil) {
		v, err = 0, nil
	}
	if err != nil {
		return 0, false
	}
	p.lastCheck.Store(time.Now().UnixNano())
	return v, true
}
