package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"

	"cloudDrive/internal/errors"
	"cloudDrive/internal/logger"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

// ErrorHandlerMiddleware 统一错误处理中间件
func ErrorHandlerMiddleware() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		requestID := GetRequestID(c)

		// 记录panic日志
		fields := &logger.LogFields{
			RequestID: requestID,
			Method:    c.Request.Method,
			Path:      c.Request.URL.Path,
			ClientIP:  c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
		}

		var errMsg string
		if err, ok := recovered.(error); ok {
			errMsg = err.Error()
		} else {
			errMsg = fmt.Sprintf("%v", recovered)
		}

		fields.Error = errMsg
		logger.Error("Panic recovered", fields, nil)

		// 记录堆栈信息
		stack := string(debug.Stack())
		logger.Error("Stack trace", &logger.LogFields{
			RequestID: requestID,
		}, fmt.Errorf("stack: %s", stack))

		// 返回统一错误响应
		isDevelopment := viper.GetString("environment") == "development"
		var details string
		if isDevelopment {
			details = sanitizeError(errMsg)
		}

		errorResp := errors.NewErrorResponse(
			errors.ErrCodeInternalServer,
			"Internal server error",
			requestID,
			details,
		)

		c.JSON(http.StatusInternalServerError, errorResp)
		c.Abort()
	})
}

// HandleError 处理应用错误
func HandleError(c *gin.Context, err error) {
	requestID := GetRequestID(c)

	fields := &logger.LogFields{
		RequestID: requestID,
		Method:    c.Request.Method,
		Path:      c.Request.URL.Path,
		ClientIP:  c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		Error:     err.Error(),
	}

	var statusCode int
	var errorResp *errors.ErrorResponse
	isDevelopment := viper.GetString("environment") == "development"

	// 判断错误类型
	if appErr, ok := err.(*errors.AppError); ok {
		statusCode = getHTTPStatusCode(appErr.Code)

		var details string
		if isDevelopment && appErr.Err != nil {
			details = sanitizeError(appErr.Err.Error())
		}

		errorResp = errors.NewErrorResponse(
			appErr.Code,
			appErr.Message,
			requestID,
			details,
		)

		logger.Error("Application error", fields, appErr.Err)
	} else {
		// 未知错误
		statusCode = http.StatusInternalServerError

		var details string
		if isDevelopment {
			details = sanitizeError(err.Error())
		}

		errorResp = errors.NewErrorResponse(
			errors.ErrCodeInternalServer,
			"Internal server error",
			requestID,
			details,
		)

		logger.Error("Unknown error", fields, err)
	}

	fields.StatusCode = statusCode
	c.JSON(statusCode, errorResp)
	c.Abort()
}

// getHTTPStatusCode 根据应用错误码获取HTTP状态码
func getHTTPStatusCode(code int) int {
	switch code {
	case errors.ErrCodeValidation:
		return http.StatusBadRequest
	case errors.ErrCodeUnauthorized:
		return http.StatusUnauthorized
	case errors.ErrCodeForbidden:
		return http.StatusForbidden
	case errors.ErrCodeNotFound:
		return http.StatusNotFound
	case errors.ErrCodeConflict:
		return http.StatusConflict
	case errors.ErrCodeRateLimit:
		return http.StatusTooManyRequests
	case errors.ErrCodeDatabaseError, errors.ErrCodeRedisError, errors.ErrCodeStorageError:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// sanitizeError 清理敏感信息
func sanitizeError(errMsg string) string {
	// 移除可能包含敏感信息的内容
	sensitivePatterns := []string{
		"password",
		"token",
		"secret",
		"key",
		"auth",
		"credential",
	}

	lowerErr := strings.ToLower(errMsg)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(lowerErr, pattern) {
			return "Error details hidden for security reasons"
		}
	}

	// 限制错误信息长度
	if len(errMsg) > 500 {
		return errMsg[:500] + "..."
	}

	return errMsg
}
