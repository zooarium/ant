// Package captcha verifies Google reCAPTCHA v3 tokens. It is deliberately
// dependency-free and decoupled from HTTP routing: callers obtain a Verifier
// and call Verify with the client-supplied token. Wire the concrete verifier
// once at startup and inject it where public write routes are mounted.
package captcha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ErrVerification is returned when a token is missing, malformed, rejected by
// Google, or scores below the configured threshold. It carries no detail so
// handlers can map it to a single 403 without leaking why.
var ErrVerification = errors.New("captcha verification failed")

// siteVerifyURL is Google's reCAPTCHA verification endpoint.
const siteVerifyURL = "https://www.google.com/recaptcha/api/siteverify"

// Verifier verifies a reCAPTCHA token for a request originating from remoteIP
// (may be empty). It returns nil when the token is valid and trusted.
type Verifier interface {
	Verify(ctx context.Context, token, remoteIP string) error
}

// noopVerifier accepts every token. Used when captcha is disabled (dev/test)
// so the rest of the pipeline behaves identically.
type noopVerifier struct{}

func (noopVerifier) Verify(context.Context, string, string) error { return nil }

// Noop returns a Verifier that accepts all tokens.
func Noop() Verifier { return noopVerifier{} }

// googleVerifier verifies tokens against Google's siteverify endpoint using a
// caller-supplied HTTP client (so the timeout comes from config, never the
// zero-timeout default client).
type googleVerifier struct {
	secret   string
	minScore float64
	client   *http.Client
}

// New returns a Verifier. When enabled is false (or secret is empty) it returns
// the noop verifier. Otherwise it returns a Google verifier that rejects tokens
// scoring below minScore. The client must be non-nil and carry a timeout.
func New(enabled bool, secret string, minScore float64, client *http.Client) Verifier {
	if !enabled || secret == "" {
		return Noop()
	}
	return &googleVerifier{secret: secret, minScore: minScore, client: client}
}

// siteVerifyResponse is the subset of Google's response we act on.
type siteVerifyResponse struct {
	Success    bool     `json:"success"`
	Score      float64  `json:"score"`
	ErrorCodes []string `json:"error-codes"`
}

func (g *googleVerifier) Verify(ctx context.Context, token, remoteIP string) error {
	if strings.TrimSpace(token) == "" {
		return ErrVerification
	}

	form := url.Values{}
	form.Set("secret", g.secret)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, siteVerifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("%w: build request: %v", ErrVerification, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: request: %v", ErrVerification, err)
	}
	defer resp.Body.Close()

	var out siteVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("%w: decode: %v", ErrVerification, err)
	}
	if !out.Success || out.Score < g.minScore {
		return ErrVerification
	}
	return nil
}
