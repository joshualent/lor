// Package apierr defines errors with a stable code and HTTP status, rendered
// by both APIs as {"error": {"code", "message"}}.
package apierr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

// Error is an API error with a stable machine-readable code.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// New returns an *Error.
func New(status int, code, format string, args ...any) *Error {
	return &Error{Status: status, Code: code, Message: fmt.Sprintf(format, args...)}
}

// Common constructors.
func BadRequest(code, format string, args ...any) *Error {
	return New(http.StatusBadRequest, code, format, args...)
}
func NotFound(code, format string, args ...any) *Error {
	return New(http.StatusNotFound, code, format, args...)
}
func Conflict(code, format string, args ...any) *Error {
	return New(http.StatusConflict, code, format, args...)
}
func Forbidden(code, format string, args ...any) *Error {
	return New(http.StatusForbidden, code, format, args...)
}
func Unauthorized(code, format string, args ...any) *Error {
	return New(http.StatusUnauthorized, code, format, args...)
}

// Code returns the error code of err if it is an *Error, else "".
func Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

type body struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Write renders err as JSON. Unknown errors become a generic 500 and are
// logged; their text is never sent to the client.
func Write(w http.ResponseWriter, err error) { WriteFor(w, nil, err) }

// WriteFor is Write with the request, which is logged with unexpected
// errors. A request the client canceled (closed tab, navigation, server
// shutdown) is normal and logged at debug level only.
func WriteFor(w http.ResponseWriter, r *http.Request, err error) {
	var e *Error
	if !errors.As(err, &e) {
		attrs := []any{"err", err}
		if r != nil {
			attrs = append(attrs, "method", r.Method, "path", r.URL.Path)
		}
		if errors.Is(err, context.Canceled) || (r != nil && r.Context().Err() != nil) {
			slog.Debug("request canceled", attrs...)
			e = New(499, "canceled", "request canceled")
		} else {
			slog.Error("internal error", attrs...)
			e = New(http.StatusInternalServerError, "internal", "internal server error")
		}
	}
	var b body
	b.Error.Code = e.Code
	b.Error.Message = e.Message
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(e.Status)
	json.NewEncoder(w).Encode(b)
}
