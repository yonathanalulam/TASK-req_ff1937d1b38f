package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/eagle-point/service-portal/internal/auth"
	"github.com/eagle-point/service-portal/internal/middleware"
	"github.com/eagle-point/service-portal/internal/models"
)

func csrfRouter(token string) *gin.Engine {
	_, r := gin.CreateTestContext(httptest.NewRecorder())

	// Inject a mock session into context (simulating RequireAuth)
	r.Use(func(c *gin.Context) {
		c.Set(auth.CtxSession, &models.Session{
			ID:        "test-session",
			CSRFToken: token,
		})
		c.Next()
	})

	csrfMW := middleware.NewCSRF(nil) // nil db — CSRF uses context only
	r.POST("/protected", csrfMW.Validate(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.GET("/public", csrfMW.Validate(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func TestCSRF_GetRequestPassesWithoutToken(t *testing.T) {
	r := csrfRouter("secret-token")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/public", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCSRF_PostWithCorrectToken(t *testing.T) {
	r := csrfRouter("secret-token")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/protected", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", "secret-token")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCSRF_PostWithoutToken_Returns400(t *testing.T) {
	r := csrfRouter("secret-token")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/protected", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCSRF_PostWithWrongToken_Returns400(t *testing.T) {
	r := csrfRouter("correct-token")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/protected", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", "wrong-token")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCSRF_PutWithCorrectToken(t *testing.T) {
	r := csrfRouter("tok")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/protected", nil)
	req.Header.Set("X-CSRF-Token", "tok")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCSRF_DeleteWithCorrectToken(t *testing.T) {
	r := csrfRouter("tok")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/protected", nil)
	req.Header.Set("X-CSRF-Token", "tok")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
