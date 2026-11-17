package response

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// SuccessEnvelope represents a standard successful JSON API response.
type SuccessEnvelope struct {
	Success   bool      `json:"success"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message,omitempty"`
	Data      any       `json:"data"`
}

// ErrorEnvelope represents a standard failure JSON API response.
type ErrorEnvelope struct {
	Success   bool      `json:"success"`
	Code      string    `json:"code"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Errors    any       `json:"errors,omitempty"`
}

func OK(c *gin.Context, data any, message ...string) {
	msg := ""
	if len(message) > 0 {
		msg = message[0]
	}
	c.JSON(http.StatusOK, SuccessEnvelope{
		Success:   true,
		Timestamp: time.Now().UTC(),
		Message:   msg,
		Data:      data,
	})
}

func Created(c *gin.Context, data any, message ...string) {
	msg := ""
	if len(message) > 0 {
		msg = message[0]
	}
	c.JSON(http.StatusCreated, SuccessEnvelope{
		Success:   true,
		Timestamp: time.Now().UTC(),
		Message:   msg,
		Data:      data,
	})
}

func JSON(c *gin.Context, status int, data any, message ...string) {
	msg := ""
	if len(message) > 0 {
		msg = message[0]
	}
	c.JSON(status, SuccessEnvelope{
		Success:   true,
		Timestamp: time.Now().UTC(),
		Message:   msg,
		Data:      data,
	})
}

func Error(c *gin.Context, appErr *AppError) {
	if appErr == nil {
		appErr = Internal("An unknown error occurred")
	}
	status := appErr.HTTPStatus
	if status == 0 {
		status = http.StatusInternalServerError
	}

	c.JSON(status, ErrorEnvelope{
		Success:   false,
		Code:      appErr.Code,
		Message:   appErr.Message,
		Timestamp: time.Now().UTC(),
		Errors:    appErr.Details,
	})
}

func HandleError(c *gin.Context, err error) {
	if err == nil {
		return
	}

	var appErr *AppError
	if errors.As(err, &appErr) {
		Error(c, appErr)
		return
	}

	Error(c, Internal("An unexpected internal error occurred", err))
}
