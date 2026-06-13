package http

import (
	"net"
	"net/http"
	"strings"

	"ant/internal/platform/render"
	"ant/pkg/captcha"
)

// CaptchaTokenHeader is the request header carrying the reCAPTCHA token.
const CaptchaTokenHeader = "X-Recaptcha-Token"

// CaptchaMiddleware returns a middleware that verifies the reCAPTCHA token on
// every request before passing it on, rejecting failures with 403. It is meant
// to wrap public write routes only — never mount it globally. When the injected
// verifier is the noop verifier (captcha disabled) it lets every request
// through, so the same wiring works in dev/test.
func CaptchaMiddleware(v captcha.Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get(CaptchaTokenHeader)
			if err := v.Verify(r.Context(), token, captchaRemoteIP(r)); err != nil {
				render.Error(w, http.StatusForbidden, "captcha verification failed")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// captchaRemoteIP extracts the originating client IP, preferring proxy headers,
// for the optional remoteip field of the siteverify call.
func captchaRemoteIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return xrip
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
