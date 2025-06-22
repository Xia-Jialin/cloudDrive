package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	RequestIDHeader = "X-Request-ID"
	RequestIDKey    = "request_id"
)

// RequestIDMiddleware 请求ID追踪中间件
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从请求头获取request_id
		requestID := c.GetHeader(RequestIDHeader)

		// 如果没有提供，则生成新的UUID
		if requestID == "" {
			requestID = generateRequestID()
		}

		// 设置到context中
		c.Set(RequestIDKey, requestID)

		// 在响应头中返回request_id
		c.Header(RequestIDHeader, requestID)

		c.Next()
	}
}

// generateRequestID 生成请求ID
func generateRequestID() string {
	return uuid.New().String()
}

// GetRequestID 从gin.Context中获取请求ID
func GetRequestID(c *gin.Context) string {
	if requestID, exists := c.Get(RequestIDKey); exists {
		if id, ok := requestID.(string); ok {
			return id
		}
	}
	return ""
}
