package policy

import (
	"context"
	"log/slog"
	"time"

	"keeper/pkg/cache"
)

// cacheKey is the single entry the TTL cache holds — the whole compiled map,
// swapped wholesale on refresh. cache.TTLCache's own locking is therefore
// sufficient; no separate mutex is needed since the map value is never
// mutated in place after Compile builds it.
const cacheKey = "policy"

// Store serves the compiled role->permission map, refreshing from falcon
// lazily on read once the TTL has expired (no background goroutine — same
// pattern as keeper/pkg/cache itself).
type Store struct {
	fetcher *Fetcher
	cache   *cache.TTLCache
}

// NewStore builds a Store that refreshes from fetcher at most once per ttl.
func NewStore(fetcher *Fetcher, ttl time.Duration) *Store {
	return &Store{fetcher: fetcher, cache: cache.New(ttl)}
}

// Warm eagerly populates the cache so the first request after boot isn't
// unguarded or slow. A failure here is logged but non-fatal — keeper must
// still be able to boot while falcon is unreachable; Policies falls back to
// a fail-closed empty map (deny-all) until a refresh succeeds.
func (s *Store) Warm(ctx context.Context) error {
	_, err := s.refresh(ctx)
	return err
}

// Policies returns the current role->policy map, refreshing from falcon if
// the cached copy has expired. Fails closed on refresh failure (past TTL, or
// at first warm) — an empty map denies every permission check rather than
// serving a stale or unpopulated one.
func (s *Store) Policies(ctx context.Context) map[string]RolePolicy {
	if v, ok := s.cache.Get(cacheKey); ok {
		return v.(map[string]RolePolicy)
	}

	m, err := s.refresh(ctx)
	if err != nil {
		slog.Error("policy cache: refresh failed, failing closed", "error", err)
		return map[string]RolePolicy{}
	}
	return m
}

func (s *Store) refresh(ctx context.Context) (map[string]RolePolicy, error) {
	rows, err := s.fetcher.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	m := Compile(rows)
	s.cache.Set(cacheKey, m)
	slog.Info("policy cache: refreshed", "roles", len(m))
	return m, nil
}
