package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/eagle-point/service-portal/internal/middleware"
)

func init() { gin.SetMode(gin.TestMode) }

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := middleware.NewRateLimiter(5, time.Minute)
	_, r := gin.CreateTestContext(httptest.NewRecorder())
	r.GET("/", rl.Limit(), func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "request %d should pass", i+1)
	}
}

func TestRateLimiter_BlocksAtLimit(t *testing.T) {
	rl := middleware.NewRateLimiter(3, time.Minute)
	_, r := gin.CreateTestContext(httptest.NewRecorder())
	r.GET("/", rl.Limit(), func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// 4th request — over limit
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.NotEmpty(t, w.Header().Get("Retry-After"))
}

func TestRateLimiter_SlidingWindow_ResetsAfterWindow(t *testing.T) {
	// Use a very short window so we can test expiry
	rl := middleware.NewRateLimiter(2, 100*time.Millisecond)
	_, r := gin.CreateTestContext(httptest.NewRecorder())
	r.GET("/", rl.Limit(), func(c *gin.Context) { c.Status(http.StatusOK) })

	// Use up 2 slots
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// 3rd should be blocked
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)

	// Should be allowed again
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
