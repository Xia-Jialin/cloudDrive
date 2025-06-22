package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRequestIDMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("generates new request ID when not provided", func(t *testing.T) {
		r := gin.New()
		r.Use(RequestIDMiddleware())
		r.GET("/test", func(c *gin.Context) {
			requestID := GetRequestID(c)
			assert.NotEmpty(t, requestID)
			c.JSON(200, gin.H{"request_id": requestID})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotEmpty(t, w.Header().Get(RequestIDHeader))
	})

	t.Run("uses provided request ID from header", func(t *testing.T) {
		providedID := "custom-request-id-123"

		r := gin.New()
		r.Use(RequestIDMiddleware())
		r.GET("/test", func(c *gin.Context) {
			requestID := GetRequestID(c)
			assert.Equal(t, providedID, requestID)
			c.JSON(200, gin.H{"request_id": requestID})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set(RequestIDHeader, providedID)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, providedID, w.Header().Get(RequestIDHeader))
	})

	t.Run("returns empty string when request ID not found in context", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		requestID := GetRequestID(c)
		assert.Empty(t, requestID)
	})

	t.Run("returns empty string when request ID is not a string", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(RequestIDKey, 123) // Set non-string value
		requestID := GetRequestID(c)
		assert.Empty(t, requestID)
	})
}
