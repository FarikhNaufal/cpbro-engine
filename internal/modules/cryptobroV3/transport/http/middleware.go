package http

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
)

// LoggerMiddleware outputs structured SRE logs utilizing slog
func LoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		rawQuery := c.Request.URL.RawQuery

		// Process request
		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		// Get scan_id or correlation context if provided in query or header
		scanID := c.Query("scan_id")
		if scanID == "" {
			scanID = c.GetHeader("X-Scan-ID")
		}

		if rawQuery != "" {
			path = path + "?" + rawQuery
		}

		// Structured logger output
		if statusCode >= 500 {
			slog.Error("HTTP request failed",
				"method", method,
				"path", path,
				"status", statusCode,
				"latency_ms", latency.Milliseconds(),
				"client_ip", clientIP,
				"scan_id", scanID,
			)
		} else if statusCode >= 400 {
			slog.Warn("HTTP request client error",
				"method", method,
				"path", path,
				"status", statusCode,
				"latency_ms", latency.Milliseconds(),
				"client_ip", clientIP,
				"scan_id", scanID,
			)
		} else {
			slog.Info("HTTP request processed",
				"method", method,
				"path", path,
				"status", statusCode,
				"latency_ms", latency.Milliseconds(),
				"client_ip", clientIP,
				"scan_id", scanID,
			)
		}
	}
}

// RecoveryMiddleware handles panics gracefully and prevents application crash
func RecoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Stack trace serialization
				stack := string(debug.Stack())
				slog.Error("HTTP panic recovered",
					"error", fmt.Sprintf("%v", err),
					"stack", stack,
				)

				c.JSON(http.StatusInternalServerError, APIResponse{
					Success: false,
					Message: "internal server error",
					Errors:  []string{"an unexpected error occurred on the server"},
				})
				c.Abort()
			}
		}()
		c.Next()
	}
}
