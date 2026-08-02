package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Envelope is the standard JSON response wrapper for all API responses.
type Envelope struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorBody  `json:"error,omitempty"`
}

// ErrorBody contains machine-readable error details.
type ErrorBody struct {
	// Code is a SCREAMING_SNAKE_CASE identifier for programmatic consumption.
	Code string `json:"code"`
	// Details carries optional structured error data (validation errors, etc.).
	Details interface{} `json:"details,omitempty"`
}

// OK sends a 200 response with the given data.
func OK(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, Envelope{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// Created sends a 201 response with the given data.
func Created(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusCreated, Envelope{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// BadRequest sends a 400 error response.
func BadRequest(c *gin.Context, message string, code string, details interface{}) {
	c.JSON(http.StatusBadRequest, Envelope{
		Success: false,
		Message: message,
		Error:   &ErrorBody{Code: code, Details: details},
	})
}

// Unauthorized sends a 401 error response.
func Unauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, Envelope{
		Success: false,
		Message: message,
		Error:   &ErrorBody{Code: "UNAUTHORIZED"},
	})
}

// Forbidden sends a 403 error response.
func Forbidden(c *gin.Context, message string) {
	c.JSON(http.StatusForbidden, Envelope{
		Success: false,
		Message: message,
		Error:   &ErrorBody{Code: "FORBIDDEN"},
	})
}

// NotFound sends a 404 error response.
func NotFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, Envelope{
		Success: false,
		Message: message,
		Error:   &ErrorBody{Code: "NOT_FOUND"},
	})
}

// TooManyRequests sends a 429 error response.
func TooManyRequests(c *gin.Context, message string) {
	c.JSON(http.StatusTooManyRequests, Envelope{
		Success: false,
		Message: message,
		Error:   &ErrorBody{Code: "RATE_LIMITED"},
	})
}

// InternalError sends a 500 error response with a generic user-facing message.
// Internal details should be logged server-side, not exposed to the client.
func InternalError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, Envelope{
		Success: false,
		Message: "Terjadi kesalahan pada server. Silakan coba lagi nanti.",
		Error:   &ErrorBody{Code: "INTERNAL_ERROR"},
	})
}

// NotImplemented sends a 501 response for endpoints that are routed but whose
// business logic has not been ported yet.
//
// This exists so an unported endpoint fails loudly instead of returning a
// fabricated success payload that a client would treat as real data. phase
// should name the migration phase that will implement it, e.g. "phase-8".
func NotImplemented(c *gin.Context, phase string) {
	c.JSON(http.StatusNotImplemented, Envelope{
		Success: false,
		Message: "Endpoint ini belum diimplementasikan pada backend Go.",
		Error: &ErrorBody{
			Code:    "NOT_IMPLEMENTED",
			Details: gin.H{"pendingPhase": phase},
		},
	})
}

// Fail sends an error response with an explicit HTTP status, message, and code.
func Fail(c *gin.Context, status int, message, code string, details interface{}) {
	c.JSON(status, Envelope{
		Success: false,
		Message: message,
		Error:   &ErrorBody{Code: code, Details: details},
	})
}

// FailedPrecondition sends a 409 response for domain rule violations.
func FailedPrecondition(c *gin.Context, message, code string) {
	Fail(c, http.StatusConflict, message, code, nil)
}

// InternalErrorWithMessage sends a 500 with a specific user-facing message.
func InternalErrorWithMessage(c *gin.Context, message string) {
	c.JSON(http.StatusInternalServerError, Envelope{
		Success: false,
		Message: message,
		Error:   &ErrorBody{Code: "INTERNAL_ERROR"},
	})
}
