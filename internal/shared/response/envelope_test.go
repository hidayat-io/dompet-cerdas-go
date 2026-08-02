package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func decode(t *testing.T, body []byte) Envelope {
	t.Helper()
	var env Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("response is not valid JSON: %v (body: %s)", err, body)
	}
	return env
}

func TestSuccessEnvelopes(t *testing.T) {
	tests := []struct {
		name       string
		call       func(*gin.Context)
		wantStatus int
	}{
		{
			name:       "OK",
			call:       func(c *gin.Context) { OK(c, "berhasil", gin.H{"id": "1"}) },
			wantStatus: http.StatusOK,
		},
		{
			name:       "Created",
			call:       func(c *gin.Context) { Created(c, "dibuat", gin.H{"id": "1"}) },
			wantStatus: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			tt.call(c)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			env := decode(t, w.Body.Bytes())
			if !env.Success {
				t.Error("success = false, want true")
			}
			if env.Error != nil {
				t.Errorf("error = %+v, want nil on a success response", env.Error)
			}
			if env.Data == nil {
				t.Error("data omitted, want present")
			}
		})
	}
}

func TestErrorEnvelopes(t *testing.T) {
	tests := []struct {
		name       string
		call       func(*gin.Context)
		wantStatus int
		wantCode   string
	}{
		{
			name:       "BadRequest",
			call:       func(c *gin.Context) { BadRequest(c, "tidak valid", "INVALID_REQUEST", nil) },
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_REQUEST",
		},
		{
			name:       "Unauthorized",
			call:       func(c *gin.Context) { Unauthorized(c, "perlu login") },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name:       "Forbidden",
			call:       func(c *gin.Context) { Forbidden(c, "dilarang") },
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name:       "NotFound",
			call:       func(c *gin.Context) { NotFound(c, "tidak ditemukan") },
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
		{
			name:       "TooManyRequests",
			call:       func(c *gin.Context) { TooManyRequests(c, "terlalu sering") },
			wantStatus: http.StatusTooManyRequests,
			wantCode:   "RATE_LIMITED",
		},
		{
			name:       "InternalError",
			call:       InternalError,
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
		{
			name:       "NotImplemented",
			call:       func(c *gin.Context) { NotImplemented(c, "phase-8") },
			wantStatus: http.StatusNotImplemented,
			wantCode:   "NOT_IMPLEMENTED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			tt.call(c)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			env := decode(t, w.Body.Bytes())
			if env.Success {
				t.Error("success = true, want false")
			}
			if env.Error == nil {
				t.Fatal("error body omitted, want present")
			}
			if env.Error.Code != tt.wantCode {
				t.Errorf("error.code = %q, want %q", env.Error.Code, tt.wantCode)
			}
			if env.Message == "" {
				t.Error("message is empty, want a user-facing message")
			}
		})
	}
}

// InternalError must not leak internal detail to the client.
func TestInternalErrorHasNoDetails(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	InternalError(c)

	env := decode(t, w.Body.Bytes())
	if env.Error.Details != nil {
		t.Errorf("error.details = %+v, want nil", env.Error.Details)
	}
}

func TestNotImplementedReportsPendingPhase(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	NotImplemented(c, "phase-9")

	env := decode(t, w.Body.Bytes())
	details, ok := env.Error.Details.(map[string]interface{})
	if !ok {
		t.Fatalf("error.details = %T, want an object", env.Error.Details)
	}
	if details["pendingPhase"] != "phase-9" {
		t.Errorf("pendingPhase = %v, want phase-9", details["pendingPhase"])
	}
}
