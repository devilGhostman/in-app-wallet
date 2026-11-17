package response

import (
	"fmt"
	"net/http"
)

const (
	CodeBadRequest          = "BAD_REQUEST"
	CodeInvalidBody         = "INVALID_BODY"
	CodeInvalidParams       = "INVALID_PARAMS"
	CodeInvalidRequest      = "INVALID_REQUEST"
	CodeNotFound            = "NOT_FOUND"
	CodeAlreadyExists       = "ALREADY_EXISTS"
	CodeNotAuthorized       = "NOT_AUTHORIZED"
	CodeForbidden           = "INSUFFICIENT_PERMISSION"
	CodeInternalServerError = "INTERNAL_SERVER_ERROR"
	CodeDatabaseError       = "DATABASE_ERROR"
	CodeServiceUnavailable  = "SERVICE_UNAVAILABLE"
	CodeTooManyRequests     = "TOO_MANY_REQUESTS"

	CodeKeyPayloadMismatch = "KEY_PAYLOAD_MISMATCH"
	CodeWalletFrozen       = "WALLET_FROZEN"
	CodeWalletClosed       = "WALLET_CLOSED"
	CodeInsufficientFunds  = "INSUFFICIENT_FUNDS"
	CodeLimitExceeded      = "LIMIT_EXCEEDED"
	CodeLockTimeout        = "LOCK_TIMEOUT"
)

// AppError represents a structured, domain-specific application error.
type AppError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
	Details    any    `json:"errors,omitempty"`
	Err        error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func NewAppError(status int, code, message string, details any, err error) *AppError {
	return &AppError{
		HTTPStatus: status,
		Code:       code,
		Message:    message,
		Details:    details,
		Err:        err,
	}
}

func BadRequest(code, message string, details ...any) *AppError {
	var det any
	if len(details) > 0 {
		det = details[0]
	}
	if code == "" {
		code = CodeBadRequest
	}
	return NewAppError(http.StatusBadRequest, code, message, det, nil)
}

func Unprocessable(code, message string, details ...any) *AppError {
	var det any
	if len(details) > 0 {
		det = details[0]
	}
	if code == "" {
		code = CodeInvalidRequest
	}
	return NewAppError(http.StatusUnprocessableEntity, code, message, det, nil)
}

func NotFound(code, message string) *AppError {
	if code == "" {
		code = CodeNotFound
	}
	return NewAppError(http.StatusNotFound, code, message, nil, nil)
}

func Internal(message string, err ...error) *AppError {
	var internalErr error
	if len(err) > 0 {
		internalErr = err[0]
	}
	if message == "" {
		message = "An unexpected internal server error occurred"
	}
	return NewAppError(http.StatusInternalServerError, CodeInternalServerError, message, nil, internalErr)
}

func ServiceUnavailable(code, message string, err ...error) *AppError {
	var internalErr error
	if len(err) > 0 {
		internalErr = err[0]
	}
	if code == "" {
		code = CodeServiceUnavailable
	}
	return NewAppError(http.StatusServiceUnavailable, code, message, nil, internalErr)
}

func Conflict(code, message string) *AppError {
	if code == "" {
		code = CodeAlreadyExists
	}
	return NewAppError(http.StatusConflict, code, message, nil, nil)
}

func Unauthorized(code, message string) *AppError {
	if code == "" {
		code = CodeNotAuthorized
	}
	return NewAppError(http.StatusUnauthorized, code, message, nil, nil)
}

func Forbidden(code, message string) *AppError {
	if code == "" {
		code = CodeForbidden
	}
	return NewAppError(http.StatusForbidden, code, message, nil, nil)
}

func TooManyRequests(code, message string) *AppError {
	if code == "" {
		code = CodeTooManyRequests
	}
	return NewAppError(http.StatusTooManyRequests, code, message, nil, nil)
}
