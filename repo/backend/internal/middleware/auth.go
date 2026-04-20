package middleware

import (
	"context"
	"database/sql"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
	"github.com/eagle-point/service-portal/internal/auth"
	"github.com/eagle-point/service-portal/internal/session"
)

// Auth provides session-based authentication middleware.
type Auth struct {
	db       *sql.DB
	sessions *session.Store
}

// NewAuth creates an Auth middleware.
func NewAuth(db *sql.DB, ss *session.Store) *Auth {
	return &Auth{db: db, sessions: ss}
}

// RequireAuth validates the session cookie, loads user roles into context,
// and extends the inactivity timer. Aborts with 401 on failure.
func (a *Auth) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessID, err := c.Cookie(session.CookieName())
		if err != nil || sessID == "" {
			apierr.Unauthorized(c)
			return
		}

		sess, err := a.sessions.GetByID(c.Request.Context(), sessID)
		if err != nil || sess == nil {
			apierr.Unauthorized(c)
			return
		}

		if sess.IsExpired() || sess.IsInactive(session.InactivityTimeout) {
			_ = a.sessions.Delete(c.Request.Context(), sessID)
			apierr.Unauthorized(c)
			return
		}

		// Load roles for this user
		rows, err := a.db.QueryContext(c.Request.Context(),
			`SELECT r.name FROM roles r JOIN user_roles ur ON ur.role_id = r.id WHERE ur.user_id = ?`,
			sess.UserID,
		)
		if err != nil {
			apierr.InternalError(c)
			return
		}
		defer rows.Close()

		var roles []string
		for rows.Next() {
			var role string
			if err := rows.Scan(&role); err == nil {
				roles = append(roles, role)
			}
		}

		// Check user is still active
		var isActive bool
		err = a.db.QueryRowContext(c.Request.Context(),
			`SELECT is_active FROM users WHERE id = ? AND is_deleted = 0`, sess.UserID,
		).Scan(&isActive)
		if err != nil || !isActive {
			apierr.Unauthorized(c)
			return
		}

		// Store session context
		c.Set(auth.CtxUserID, sess.UserID)
		c.Set(auth.CtxRoles, roles)
		c.Set(auth.CtxSession, sess)

		// Extend inactivity timer. Previously this was goroutine'd with
		// c.Request.Context() — which meant it frequently raced with request
		// completion and ran against a cancelled context, silently failing to
		// update last_active_at and triggering false inactivity expiry for
		// genuinely active users. Now it uses a bounded background context
		// that survives request teardown and has a hard timeout so a slow DB
		// doesn't leak goroutines.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = a.sessions.Touch(ctx, sessID)
		}()

		c.Next()
	}
}
