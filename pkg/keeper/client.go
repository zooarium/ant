// Package keeper is a minimal client for keeper's public s2s surfaces, used to
// enrich ant responses with tenant (app) details.
package keeper

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ant/pkg/cache"
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
	http  *http.Client
	base  string
	cache *cache.TTLCache
}

// NewClient builds a Client. httpClient must carry a non-zero timeout.
func NewClient(httpClient *http.Client, baseURL string, ttl time.Duration) *Client {
	return &Client{
		http:  httpClient,
		base:  strings.TrimRight(baseURL, "/"),
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
	url := fmt.Sprintf("%s/apps/%d/public", c.base, appID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Warn("keeper app profile: build request failed", "app_id", appID, "error", err)
		return nil
	}

	resp, err := c.http.Do(req)
	if err != nil {
		slog.Warn("keeper app profile: request failed", "app_id", appID, "error", err)
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("keeper app profile: non-200", "app_id", appID, "status", resp.StatusCode)
		return nil
	}

	var body struct {
		Data AppProfile `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		slog.Warn("keeper app profile: decode failed", "app_id", appID, "error", err)
		return nil
	}
	return &body.Data
}
