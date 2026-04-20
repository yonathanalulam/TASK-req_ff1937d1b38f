package moderation

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
	"github.com/eagle-point/service-portal/internal/auth"
)

// Gin context keys set when ScreenContent finds a borderline match.
const (
	ScreenResultKey  = "ctx_screen_result"
	BorderlineTextKey  = "ctx_borderline_text"
	BorderlineTermsKey = "ctx_borderline_terms"
)

// FreezeCheck aborts with 403 if the authenticated user is under a posting freeze.
func (s *Service) FreezeCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetUint64(auth.CtxUserID)
		if userID == 0 {
			c.Next()
			return
		}
		until, err := s.IsUserFrozen(c.Request.Context(), userID)
		if err == nil && until != nil {
			apierr.ForbiddenWith(c, "posting_frozen",
				"posting is temporarily disabled due to content-policy violations",
				map[string]any{"freeze_until": until})
			return
		}
		c.Next()
	}
}

// ScreenContent scans the JSON body of the request against the dictionary.
// `fields` lists the JSON keys to concatenate and screen.
//
// Behavior:
//   - Prohibited match → abort 422 with code "content_blocked"
//   - Borderline match → let the request through but stash the result so the
//     handler can persist the content with status='pending_moderation' and
//     enqueue a moderation_queue entry afterward.
//   - No match → continue normally with no context value set.
//
// The middleware reads and rewinds the request body, so downstream ShouldBindJSON
// calls still work as expected.
func (s *Service) ScreenContent(fields ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil {
			c.Next()
			return
		}
		// Only screen JSON bodies — multipart is handled at the handler level.
		if !strings.HasPrefix(c.ContentType(), "application/json") {
			c.Next()
			return
		}

		raw, err := io.ReadAll(c.Request.Body)
		if err != nil {
			apierr.BadRequest(c, "invalid_body", "could not read request body")
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(raw))

		var parsed map[string]any
		if err := json.Unmarshal(raw, &parsed); err != nil {
			// Not JSON or malformed → let downstream handler return its own error.
			c.Next()
			return
		}

		var joined strings.Builder
		for _, f := range fields {
			if v, ok := parsed[f]; ok {
				if str, ok := v.(string); ok {
					joined.WriteString(str)
					joined.WriteByte(' ')
				}
			}
		}
		text := joined.String()
		if text == "" {
			c.Next()
			return
		}

		result := s.Screen(c.Request.Context(), text)
		if result.HasProhibited() {
			apierr.UnprocessableEntity(c, "content_blocked",
				"content contains prohibited terms",
				map[string]any{"flagged_terms": result.FlaggedTerms})
			return
		}
		if result.HasBorderline() {
			c.Set(ScreenResultKey, result)
			// Mirror primitives so handlers can read without importing moderation.
			c.Set(BorderlineTextKey, text)
			c.Set(BorderlineTermsKey, result.FlaggedTerms)
		}
		c.Next()
	}
}

// GetScreenResult extracts the borderline screening result if one was set.
// Handlers call this after their content insert to know whether to enqueue the row.
func GetScreenResult(c *gin.Context) (ScreenResult, bool) {
	if v, ok := c.Get(ScreenResultKey); ok {
		if r, ok := v.(ScreenResult); ok {
			return r, true
		}
	}
	return ScreenResult{}, false
}

