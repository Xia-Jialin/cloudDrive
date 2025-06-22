package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cloudDrive/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestHealthCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("basic health check without dependencies", func(t *testing.T) {
		r := gin.New()
		r.Use(middleware.RequestIDMiddleware())
		r.GET("/health", HealthCheck)

		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response EnhancedHealthCheckResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		assert.Equal(t, StatusHealthy, response.Status)
		assert.Equal(t, "1.0.0", response.Version)
		assert.NotEmpty(t, response.RequestID)
		assert.NotEmpty(t, response.Uptime)
		assert.NotEmpty(t, response.Timestamp)
		assert.NotNil(t, response.SystemInfo)
		assert.NotNil(t, response.Components)

		// 验证系统信息
		assert.Greater(t, response.SystemInfo.CPUCount, 0)
		assert.Greater(t, response.SystemInfo.GoroutineCount, 0)
		assert.Greater(t, response.SystemInfo.MemoryUsage.Alloc, uint64(0))

		// 验证响应头包含请求ID
		assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
	})

	t.Run("health check with custom request ID", func(t *testing.T) {
		customRequestID := "custom-health-check-id"

		r := gin.New()
		r.Use(middleware.RequestIDMiddleware())
		r.GET("/health", HealthCheck)

		req := httptest.NewRequest("GET", "/health", nil)
		req.Header.Set("X-Request-ID", customRequestID)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response EnhancedHealthCheckResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		assert.Equal(t, customRequestID, response.RequestID)
		assert.Equal(t, customRequestID, w.Header().Get("X-Request-ID"))
	})
}

func TestComponentStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   ComponentStatus
		expected string
	}{
		{"healthy status", StatusHealthy, "healthy"},
		{"degraded status", StatusDegraded, "degraded"},
		{"unhealthy status", StatusUnhealthy, "unhealthy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.status))
		})
	}
}

func TestComponent(t *testing.T) {
	component := Component{
		Status:       StatusHealthy,
		Message:      "Service is running",
		ResponseTime: "10ms",
		Details:      map[string]interface{}{"version": "1.0.0"},
	}

	assert.Equal(t, StatusHealthy, component.Status)
	assert.Equal(t, "Service is running", component.Message)
	assert.Equal(t, "10ms", component.ResponseTime)
	assert.NotNil(t, component.Details)
}
