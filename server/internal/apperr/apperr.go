// Package apperr defines domain errors with stable codes for the UI.
package apperr

import (
	"errors"
	"net/http"
)

// Error carries an HTTP status, a stable code and an English fallback message.
type Error struct {
	Status int
	Code   string
	Msg    string
}

func (e *Error) Error() string { return e.Code + ": " + e.Msg }

// Common errors.
var (
	ErrUnauthorized   = &Error{http.StatusUnauthorized, "auth.unauthorized", "missing or invalid token"}
	ErrForbidden      = &Error{http.StatusForbidden, "auth.forbidden", "not allowed"}
	ErrNotFound       = &Error{http.StatusNotFound, "common.not_found", "resource not found"}
	ErrBadRequest     = &Error{http.StatusBadRequest, "common.bad_request", "invalid request"}
	ErrBadPassword    = &Error{http.StatusUnauthorized, "auth.bad_password", "wrong access code"}
	ErrUnknownAccount = &Error{http.StatusForbidden, "auth.unknown_account", "sign-up is disabled on this server"}
	ErrDevLoginOff    = &Error{http.StatusForbidden, "auth.dev_login_disabled", "dev login is disabled"}
	ErrIllegalState   = &Error{http.StatusConflict, "task.illegal_transition", "task cannot change to that state"}
	ErrDaemonUnknown  = &Error{http.StatusBadRequest, "task.daemon_unknown", "daemon does not belong to this workspace"}
	ErrAlreadyDone    = &Error{http.StatusConflict, "common.already_decided", "already decided"}
)

// BadRequest builds a 400 with a custom message.
func BadRequest(msg string) *Error {
	return &Error{http.StatusBadRequest, "common.bad_request", msg}
}

// Forbidden builds a 403 with a custom code.
func Forbidden(code, msg string) *Error {
	return &Error{http.StatusForbidden, code, msg}
}

// Conflict builds a 409 with a custom code.
func Conflict(code, msg string) *Error {
	return &Error{http.StatusConflict, code, msg}
}

// Delegation builds a 422 carrying a delegation reason code.
func Delegation(code, msg string) *Error {
	return &Error{http.StatusUnprocessableEntity, "delegation." + code, msg}
}

// As extracts an *Error or wraps unknown errors as 500.
func As(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{http.StatusInternalServerError, "common.internal", "internal error"}
}

// ErrReasonRequired is returned when a rejection has no reason (FR-CLI-303).
var ErrReasonRequired = &Error{http.StatusBadRequest, "common.reason_required", "a reason is required to reject"}
