package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// HealthCheckResponse 健康检查响应
type HealthCheckResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
	Uptime    string    `json:"uptime"`
}

var startTime = time.Now()

// HealthCheck 处理健康检查请求
func HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, HealthCheckResponse{
		Status:    "ok",
		Timestamp: time.Now(),
		Version:   "1.0.0",
		Uptime:    time.Since(startTime).String(),
	})
}
