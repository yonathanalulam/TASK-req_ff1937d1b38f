package apierr

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// envelope is the standard error response shape:
//
//	{ "error": { "code": "...", "message": "...", "details": {...} } }
type envelope struct {
	Error detail `json:"error"`
}

type detail struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func respond(c *gin.Context, status int, code, message string, details map[string]any) {
	c.AbortWithStatusJSON(status, envelope{
		Error: detail{Code: code, Message: message, Details: details},
	})
}

// BadRequest sends a 400 response.
func BadRequest(c *gin.Context, code, message string) {
	respond(c, http.StatusBadRequest, code, message, nil)
}

// Unauthorized sends a 401 response.
func Unauthorized(c *gin.Context) {
	respond(c, http.StatusUnauthorized, "unauthorized", "authentication required", nil)
}

// Forbidden sends a 403 response.
func Forbidden(c *gin.Context) {
	respond(c, http.StatusForbidden, "forbidden", "insufficient permissions", nil)
}

// ForbiddenWith sends a 403 response with a custom code, message, and optional details.
// Used for cases like posting freezes where callers need structured metadata in the body.
func ForbiddenWith(c *gin.Context, code, message string, details map[string]any) {
	respond(c, http.StatusForbidden, code, message, details)
}

// NotFound sends a 404 response.
func NotFound(c *gin.Context, entity string) {
	respond(c, http.StatusNotFound, "not_found", entity+" not found", nil)
}

// Conflict sends a 409 response.
func Conflict(c *gin.Context, code, message string) {
	respond(c, http.StatusConflict, code, message, nil)
}

// UnprocessableEntity sends a 422 response.
func UnprocessableEntity(c *gin.Context, code, message string, details map[string]any) {
	respond(c, http.StatusUnprocessableEntity, code, message, details)
}

// TooManyRequests sends a 429 response with Retry-After header.
func TooManyRequests(c *gin.Context, retryAfterSeconds int) {
	c.Header("Retry-After", fmt.Sprint(retryAfterSeconds))
	respond(c, http.StatusTooManyRequests, "rate_limited", "too many requests", map[string]any{
		"retry_after_seconds": retryAfterSeconds,
	})
}

// AccountLocked sends a 403 with lockout details.
func AccountLocked(c *gin.Context, remainingSeconds int) {
	respond(c, http.StatusForbidden, "account_locked",
		"account temporarily locked due to multiple failed login attempts",
		map[string]any{"remaining_seconds": remainingSeconds})
}

// InternalError sends a 500 response.
func InternalError(c *gin.Context) {
	respond(c, http.StatusInternalServerError, "internal_error", "an unexpected error occurred", nil)
}
