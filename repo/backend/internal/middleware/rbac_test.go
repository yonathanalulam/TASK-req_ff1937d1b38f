package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/eagle-point/service-portal/internal/auth"
	"github.com/eagle-point/service-portal/internal/middleware"
	"github.com/eagle-point/service-portal/internal/models"
)

func setupRBACRouter(roles []string) *gin.Engine {
	_, r := gin.CreateTestContext(httptest.NewRecorder())

	// Inject roles into context (simulating RequireAuth having run)
	r.Use(func(c *gin.Context) {
		c.Set(auth.CtxRoles, roles)
		c.Next()
	})

	r.GET("/admin",
		middleware.RequireRole(models.RoleAdministrator),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)
	r.GET("/agent",
		middleware.RequireRole(models.RoleServiceAgent, models.RoleAdministrator),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)
	r.GET("/any",
		middleware.RequireRole(models.RoleRegularUser, models.RoleServiceAgent,
			models.RoleModerator, models.RoleAdministrator, models.RoleDataOperator),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)
	return r
}

func TestRequireRole_AllowsMatchingRole(t *testing.T) {
	r := setupRBACRouter([]string{models.RoleAdministrator})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireRole_BlocksMismatchedRole(t *testing.T) {
	r := setupRBACRouter([]string{models.RoleRegularUser})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireRole_AllowsMultiRoleMatch(t *testing.T) {
	r := setupRBACRouter([]string{models.RoleServiceAgent})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/agent", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireRole_BlocksEmptyRoles(t *testing.T) {
	r := setupRBACRouter([]string{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireRole_BlocksNoContextSet(t *testing.T) {
	_, r := gin.CreateTestContext(httptest.NewRecorder())
	// No middleware sets CtxRoles
	r.GET("/admin",
		middleware.RequireRole(models.RoleAdministrator),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireRole_EachRoleAccessMatrix(t *testing.T) {
	cases := []struct {
		role    string
		path    string
		wantOK  bool
	}{
		{models.RoleAdministrator, "/admin", true},
		{models.RoleRegularUser, "/admin", false},
		{models.RoleModerator, "/admin", false},
		{models.RoleDataOperator, "/admin", false},
		{models.RoleServiceAgent, "/agent", true},
		{models.RoleAdministrator, "/agent", true},
		{models.RoleRegularUser, "/agent", false},
		{models.RoleRegularUser, "/any", true},
		{models.RoleModerator, "/any", true},
	}

	for _, tc := range cases {
		r := setupRBACRouter([]string{tc.role})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, tc.path, nil)
		r.ServeHTTP(w, req)

		if tc.wantOK {
			assert.Equal(t, http.StatusOK, w.Code, "role=%s path=%s should be allowed", tc.role, tc.path)
		} else {
			assert.Equal(t, http.StatusForbidden, w.Code, "role=%s path=%s should be forbidden", tc.role, tc.path)
		}
	}
}
