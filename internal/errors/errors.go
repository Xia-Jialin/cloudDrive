package errors

import (
	"fmt"
	"time"
)

// 错误码定义
const (
	ErrCodeSuccess        = 0
	ErrCodeInternalServer = 50000
	ErrCodeDatabaseError  = 50001
	ErrCodeRedisError     = 50002
	ErrCodeStorageError   = 50003
	ErrCodeValidation     = 40001
	ErrCodeUnauthorized   = 40100
	ErrCodeForbidden      = 40300
	ErrCodeNotFound       = 40400
	ErrCodeConflict       = 40900
	ErrCodeRateLimit      = 42900
)

// ErrorResponse 统一错误响应格式
type ErrorResponse struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Timestamp string `json:"timestamp"`
	Details   string `json:"details,omitempty"` // 仅开发环境显示
}

// AppError 应用错误结构
type AppError struct {
	Code    int
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("code: %d, message: %s, error: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("code: %d, message: %s", e.Code, e.Message)
}

// NewAppError 创建应用错误
func NewAppError(code int, message string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// NewErrorResponse 创建错误响应
func NewErrorResponse(code int, message, requestID string, details ...string) *ErrorResponse {
	resp := &ErrorResponse{
		Code:      code,
		Message:   message,
		RequestID: requestID,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if len(details) > 0 {
		resp.Details = details[0]
	}

	return resp
}

// 预定义错误
var (
	ErrInternalServer = NewAppError(ErrCodeInternalServer, "Internal server error", nil)
	ErrDatabaseError  = NewAppError(ErrCodeDatabaseError, "Database error", nil)
	ErrRedisError     = NewAppError(ErrCodeRedisError, "Redis error", nil)
	ErrStorageError   = NewAppError(ErrCodeStorageError, "Storage error", nil)
	ErrValidation     = NewAppError(ErrCodeValidation, "Validation error", nil)
	ErrUnauthorized   = NewAppError(ErrCodeUnauthorized, "Unauthorized", nil)
	ErrForbidden      = NewAppError(ErrCodeForbidden, "Forbidden", nil)
	ErrNotFound       = NewAppError(ErrCodeNotFound, "Not found", nil)
	ErrConflict       = NewAppError(ErrCodeConflict, "Conflict", nil)
	ErrRateLimit      = NewAppError(ErrCodeRateLimit, "Rate limit exceeded", nil)
)
