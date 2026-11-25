package services

import (
	"errors"
	"fmt"
	"net/http"
)

// AppError is the only error type handlers translate. Anything else becomes a
// 500 with a generic message, so internal details never leak to clients.
type AppError struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func (e *AppError) Error() string { return e.Message }

func NewError(status int, code, message string) *AppError {
	return &AppError{Status: status, Code: code, Message: message}
}

func BadRequest(message string) *AppError {
	return NewError(http.StatusBadRequest, "bad_request", message)
}
func Unauthorized(message string) *AppError {
	return NewError(http.StatusUnauthorized, "unauthorized", message)
}
func Forbidden(message string) *AppError { return NewError(http.StatusForbidden, "forbidden", message) }
func NotFound(message string) *AppError  { return NewError(http.StatusNotFound, "not_found", message) }
func Conflict(message string) *AppError  { return NewError(http.StatusConflict, "conflict", message) }

func Validation(fields map[string]string) *AppError {
	return &AppError{
		Status:  http.StatusUnprocessableEntity,
		Code:    "validation_failed",
		Message: "please check the highlighted fields",
		Fields:  fields,
	}
}

func Upstream(err error) *AppError {
	return NewError(http.StatusBadGateway, "upstream_error", fmt.Sprintf("%v", err))
}

func AsAppError(err error) *AppError {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	return NewError(http.StatusInternalServerError, "internal_error", "internal server error")
}
