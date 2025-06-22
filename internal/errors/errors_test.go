package errors

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAppError(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		message  string
		err      error
		expected string
	}{
		{
			name:     "with underlying error",
			code:     ErrCodeDatabaseError,
			message:  "Database connection failed",
			err:      fmt.Errorf("connection timeout"),
			expected: "code: 50001, message: Database connection failed, error: connection timeout",
		},
		{
			name:     "without underlying error",
			code:     ErrCodeValidation,
			message:  "Invalid input",
			err:      nil,
			expected: "code: 40001, message: Invalid input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appErr := NewAppError(tt.code, tt.message, tt.err)
			assert.Equal(t, tt.expected, appErr.Error())
			assert.Equal(t, tt.code, appErr.Code)
			assert.Equal(t, tt.message, appErr.Message)
			assert.Equal(t, tt.err, appErr.Err)
		})
	}
}

func TestNewErrorResponse(t *testing.T) {
	requestID := "test-request-id"
	code := ErrCodeInternalServer
	message := "Internal server error"
	details := "Detailed error information"

	resp := NewErrorResponse(code, message, requestID, details)

	assert.Equal(t, code, resp.Code)
	assert.Equal(t, message, resp.Message)
	assert.Equal(t, requestID, resp.RequestID)
	assert.Equal(t, details, resp.Details)
	assert.NotEmpty(t, resp.Timestamp)

	// 验证时间戳格式
	_, err := time.Parse(time.RFC3339, resp.Timestamp)
	assert.NoError(t, err)
}

func TestNewErrorResponseWithoutDetails(t *testing.T) {
	requestID := "test-request-id"
	code := ErrCodeNotFound
	message := "Resource not found"

	resp := NewErrorResponse(code, message, requestID)

	assert.Equal(t, code, resp.Code)
	assert.Equal(t, message, resp.Message)
	assert.Equal(t, requestID, resp.RequestID)
	assert.Empty(t, resp.Details)
}

func TestPredefinedErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     *AppError
		code    int
		message string
	}{
		{
			name:    "ErrInternalServer",
			err:     ErrInternalServer,
			code:    ErrCodeInternalServer,
			message: "Internal server error",
		},
		{
			name:    "ErrDatabaseError",
			err:     ErrDatabaseError,
			code:    ErrCodeDatabaseError,
			message: "Database error",
		},
		{
			name:    "ErrRedisError",
			err:     ErrRedisError,
			code:    ErrCodeRedisError,
			message: "Redis error",
		},
		{
			name:    "ErrValidation",
			err:     ErrValidation,
			code:    ErrCodeValidation,
			message: "Validation error",
		},
		{
			name:    "ErrUnauthorized",
			err:     ErrUnauthorized,
			code:    ErrCodeUnauthorized,
			message: "Unauthorized",
		},
		{
			name:    "ErrNotFound",
			err:     ErrNotFound,
			code:    ErrCodeNotFound,
			message: "Not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.code, tt.err.Code)
			assert.Equal(t, tt.message, tt.err.Message)
			assert.Nil(t, tt.err.Err)
		})
	}
}
