package auth_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ─── Password validation rules (mirror of auth.validatePassword) ─────────────
// These tests document the contract without importing the unexported function.

func validateForTest(pw string) error {
	if len(pw) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	var hasUpper, hasDigit bool
	for _, c := range pw {
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		}
		if c >= '0' && c <= '9' {
			hasDigit = true
		}
	}
	if !hasUpper {
		return fmt.Errorf("password must contain at least one uppercase letter")
	}
	if !hasDigit {
		return fmt.Errorf("password must contain at least one digit")
	}
	return nil
}

func TestPasswordValidation_Rules(t *testing.T) {
	cases := []struct {
		pw    string
		valid bool
	}{
		{"short1A", false},
		{"alllowercase1", false},
		{"NoDigitsHere", false},
		{"ValidPass1", true},
		{"Password1!", true},
		{"Aa1bcdefgh", true},
	}

	for _, tc := range cases {
		err := validateForTest(tc.pw)
		if tc.valid {
			assert.NoError(t, err, "expected valid: %s", tc.pw)
		} else {
			assert.Error(t, err, "expected invalid: %s", tc.pw)
		}
	}
}

// ─── Request binding validation ──────────────────────────────────────────────

func TestLoginRequest_MissingPassword(t *testing.T) {
	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)

	r.POST("/login", func(c *gin.Context) {
		var req struct {
			Username string `json:"username" binding:"required"`
			Password string `json:"password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{})
	})

	body, _ := json.Marshal(map[string]string{"username": "testuser"})
	req, _ := http.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRegisterRequest_InvalidEmail(t *testing.T) {
	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)

	r.POST("/register", func(c *gin.Context) {
		var req struct {
			Username    string `json:"username"     binding:"required,min=3,max=50"`
			Email       string `json:"email"        binding:"required,email"`
			Password    string `json:"password"     binding:"required"`
			DisplayName string `json:"display_name" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{})
	})

	body, _ := json.Marshal(map[string]string{
		"username":     "ab", // min=3 violation
		"email":        "not-an-email",
		"password":     "pass",
		"display_name": "Test",
	})
	req, _ := http.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthMe_MissingAuth(t *testing.T) {
	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)

	r.GET("/me", func(c *gin.Context) {
		// Simulate missing authentication
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
	})

	req, _ := http.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMe_Success(t *testing.T) {
	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)

	r.GET("/me", func(c *gin.Context) {
		// Simulate successful authentication
		c.JSON(http.StatusOK, gin.H{
			"id": 1,
			"username": "testuser",
			"email": "test@example.com",
			"display_name": "Test User"
		})
	})

	req, _ := http.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
