package health

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response is returned by the health endpoint.
type Response struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

// Handler returns a Gin handler that reports service health.
func Handler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		dbStatus := "ok"
		if err := db.Ping(); err != nil {
			dbStatus = "unreachable"
			c.JSON(http.StatusServiceUnavailable, Response{
				Status:   "degraded",
				Database: dbStatus,
			})
			return
		}

		c.JSON(http.StatusOK, Response{
			Status:   "ok",
			Database: dbStatus,
		})
	}
}
