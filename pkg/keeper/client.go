// Package keeper is a minimal client for keeper's public s2s surfaces, used to
// enrich ant responses with tenant (app) details.
package keeper

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"keeper/pkg/cache"
	"keeper/pkg/s2s"
)

// Address mirrors keeper's public app address.
type Address struct {
	Line1      string `json:"line1"`
	Line2      string `json:"line2"`
	City       string `json:"city"`
	State      string `json:"state"`
	Country    string `json:"country"`
	PostalCode string `json:"postal_code"`
}

// Contact mirrors keeper's public app contact details.
type Contact struct {
	Address Address `json:"address"`
	Phone1  string  `json:"phone1"`
	Phone2  string  `json:"phone2"`
	Email   string  `json:"email"`
}

// AppProfile is the public-safe app profile served by keeper
// GET /apps/{id}/public.
type AppProfile struct {
	ID         int     `json:"id"`
	Name       string  `json:"name"`
	Tagline    string  `json:"tagline"`
	LogoURL    string  `json:"logo_url"`
	TaxNumber  string  `json:"tax_number"`
	TaxPercent float64 `json:"tax_percent"`
	Contact    Contact `json:"contact"`
}

// Client fetches app profiles from keeper with an in-memory TTL cache in
// front, so the hot order-read path stays cheap and keeper's rate-limited
// public endpoint is not hammered.
type Client struct {
	rest  *s2s.Client
	cache *cache.TTLCache
}

// NewClient builds a Client. httpClient must carry a non-zero timeout.
func NewClient(httpClient *http.Client, baseURL string, ttl time.Duration) *Client {
	return &Client{
		rest:  s2s.New(httpClient, baseURL),
		cache: cache.New(ttl),
	}
}

// AppProfile returns keeper's public profile for appID, or nil when keeper is
// unreachable or the app is unknown/inactive. Results — including misses — are
// cached for the client TTL, so a keeper outage degrades to unenriched
// responses rather than a per-request outbound call. Enrichment is optional;
// this never fails the caller.
func (c *Client) AppProfile(ctx context.Context, appID int) *AppProfile {
	key := strconv.Itoa(appID)
	if v, ok := c.cache.Get(key); ok {
		p, _ := v.(*AppProfile)
		return p
	}

	p := c.fetch(ctx, appID)
	c.cache.Set(key, p)
	return p
}

func (c *Client) fetch(ctx context.Context, appID int) *AppProfile {
	var profile AppProfile
	if err := c.rest.Get(ctx, fmt.Sprintf("/apps/%d/public", appID), &profile); err != nil {
		slog.Warn("keeper app profile: fetch failed", "app_id", appID, "error", err)
		return nil
	}
	return &profile
}
