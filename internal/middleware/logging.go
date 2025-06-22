package middleware

import (
	"time"

	"cloudDrive/internal/logger"

	"github.com/gin-gonic/gin"
)

// LoggingMiddleware HTTP请求日志记录中间件
func LoggingMiddleware() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		// 不返回字符串，而是使用结构化日志
		return ""
	})
}

// StructuredLoggingMiddleware 结构化日志中间件
func StructuredLoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 记录开始时间
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// 处理请求
		c.Next()

		// 计算处理时间
		duration := time.Since(start)

		// 获取用户ID（如果存在）
		var userID string
		if user, exists := c.Get("user_id"); exists {
			if uid, ok := user.(string); ok {
				userID = uid
			}
		}

		// 构建日志字段
		fields := &logger.LogFields{
			RequestID:  GetRequestID(c),
			UserID:     userID,
			Method:     c.Request.Method,
			Path:       path,
			StatusCode: c.Writer.Status(),
			Duration:   duration,
			ClientIP:   c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
		}

		// 添加查询参数到路径
		if raw != "" {
			fields.Path = path + "?" + raw
		}

		// 根据状态码决定日志级别
		if c.Writer.Status() >= 500 {
			logger.Error("HTTP request completed with server error", fields, nil)
		} else if c.Writer.Status() >= 400 {
			logger.Warn("HTTP request completed with client error", fields)
		} else {
			logger.Info("HTTP request completed", fields)
		}
	}
}

// SkipLoggingMiddleware 跳过某些路径的日志记录
func SkipLoggingMiddleware(skipPaths ...string) gin.HandlerFunc {
	skipMap := make(map[string]bool)
	for _, path := range skipPaths {
		skipMap[path] = true
	}

	return func(c *gin.Context) {
		if skipMap[c.Request.URL.Path] {
			c.Next()
			return
		}

		StructuredLoggingMiddleware()(c)
	}
}
