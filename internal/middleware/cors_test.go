package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func corsRouter() *gin.Engine {
	r := gin.New()
	r.Use(CORS([]string{"https://app.example.com", "https://alt.example.com/"}))
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	r.OPTIONS("/ping", func(c *gin.Context) { c.String(http.StatusOK, "should not reach") })
	return r
}

func TestCORS_AllowedOriginGetsHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://app.example.com")

	corsRouter().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("Allow-Origin = %q, want the request origin", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Allow-Credentials = %q, want true", got)
	}
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want Origin", got)
	}
}

// A trailing slash in the configured origin must not prevent a match, since
// browsers never send one in the Origin header.
func TestCORS_TrailingSlashInConfigIsNormalised(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://alt.example.com")

	corsRouter().ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://alt.example.com" {
		t.Errorf("Allow-Origin = %q, want the request origin", got)
	}
}

func TestCORS_DisallowedOriginGetsNoHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://evil.example.com")

	corsRouter().ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want empty for a disallowed origin", got)
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (the request itself still runs)", w.Code)
	}
}

// A wildcard origin combined with credentials is rejected by browsers, so it
// must never be emitted.
func TestCORS_NeverEmitsWildcard(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://app.example.com")

	corsRouter().ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got == "*" {
		t.Error("Allow-Origin = *, which browsers reject alongside credentials")
	}
}

func TestCORS_PreflightAllowedOrigin(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "https://app.example.com")

	corsRouter().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("Allow-Methods missing on preflight")
	}
	if body := w.Body.String(); body != "" {
		t.Errorf("preflight body = %q, want empty (handler must not run)", body)
	}
}

func TestCORS_PreflightDisallowedOriginIsForbidden(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "https://evil.example.com")

	corsRouter().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("preflight status = %d, want 403", w.Code)
	}
}

func TestCORS_NoOriginHeaderPassesThrough(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)

	corsRouter().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for a same-origin/server-side request", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want empty when no Origin was sent", got)
	}
}
