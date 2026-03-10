package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type CORSMiddleware struct {
	allowedOrigins map[string]struct{}
}

func NewCORSMiddleware(origins []string) *CORSMiddleware {
	allowedOrigins := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		if origin == "" {
			continue
		}
		allowedOrigins[origin] = struct{}{}
	}

	return &CORSMiddleware{
		allowedOrigins: allowedOrigins,
	}
}

func (m *CORSMiddleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		if _, ok := m.allowedOrigins[origin]; !ok {
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
			return
		}

		headers := c.Writer.Header()
		headers.Set("Access-Control-Allow-Origin", origin)
		headers.Set("Vary", "Origin")
		headers.Set("Access-Control-Allow-Credentials", "true")
		headers.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		headers.Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, Origin, X-Requested-With")
		headers.Set("Access-Control-Expose-Headers", "Content-Length, Content-Type")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
