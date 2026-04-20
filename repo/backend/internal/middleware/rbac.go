package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
	"github.com/eagle-point/service-portal/internal/auth"
)

// RequireRole returns a middleware that aborts with 403 unless the authenticated
// user holds at least one of the given roles.
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}

	return func(c *gin.Context) {
		userRoles, exists := c.Get(auth.CtxRoles)
		if !exists {
			apierr.Forbidden(c)
			return
		}

		list, ok := userRoles.([]string)
		if !ok {
			apierr.Forbidden(c)
			return
		}

		for _, r := range list {
			if _, ok := allowed[r]; ok {
				c.Next()
				return
			}
		}

		apierr.Forbidden(c)
	}
}
