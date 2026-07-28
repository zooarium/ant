package policy

import (
	"context"
	"fmt"
	"net/http"

	"keeper/pkg/auth"
	"keeper/pkg/s2s"
)

// Fetcher pulls the current role-permission export for one falcon service.
type Fetcher struct {
	client    *s2s.Client
	jwt       *auth.JWTManager
	serviceID int
}

// NewFetcher builds a Fetcher bound to falcon's internal-s2s base URL.
// httpClient must carry a non-zero timeout (keeper/pkg/httpclient). jwt
// self-signs the s2s call — the internal-s2s listener only checks the
// signature against the shared AUTH.JWT_SECRET, not the claims, so any
// valid token works.
func NewFetcher(httpClient *http.Client, falconBaseURL string, serviceID int, jwt *auth.JWTManager) *Fetcher {
	return &Fetcher{client: s2s.New(httpClient, falconBaseURL), jwt: jwt, serviceID: serviceID}
}

// Fetch calls GET /services/{id}/permissions/map and returns the raw rows.
func (f *Fetcher) Fetch(ctx context.Context) ([]Row, error) {
	token, err := f.jwt.Generate(0, 0, 0, auth.RoleSysAdmin)
	if err != nil {
		return nil, fmt.Errorf("sign s2s token: %w", err)
	}

	var rows []Row
	path := fmt.Sprintf("/services/%d/permissions/map", f.serviceID)
	if err := f.client.GetAuth(ctx, path, token, &rows); err != nil {
		return nil, fmt.Errorf("falcon permissions map: %w", err)
	}
	return rows, nil
}
