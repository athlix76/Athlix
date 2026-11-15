// Package httpx holds the HTTP conventions every handler follows: the JSON envelope,
// typed API errors, request binding and cursor pagination. See docs/api.md "Conventions".
package httpx

import (
	"errors"
	"fmt"
	"net/http"
)

// Error codes are a closed list; clients switch on Code, never on Message.
const (
	CodeBadRequest       = "BAD_REQUEST"
	CodeValidation       = "VALIDATION_FAILED"
	CodeUnauthenticated  = "UNAUTHENTICATED"
	CodeForbidden        = "FORBIDDEN"
	CodeNotFound         = "NOT_FOUND"
	CodeConflict         = "CONFLICT"
	CodeRateLimited      = "RATE_LIMITED"
	CodeOnboarding       = "ONBOARDING_REQUIRED"
	CodeVerification     = "VERIFICATION_REQUIRED"
	CodeOrganiserPending = "ORGANISER_NOT_APPROVED"
	CodePlanRequired     = "PLAN_REQUIRED"
	CodeLimitReached     = "LIMIT_REACHED"
	CodeInvalidState     = "INVALID_STATE"
	CodeSuspended        = "ACCOUNT_SUSPENDED"
	CodeMaintenance      = "MAINTENANCE"
	CodeUpstream         = "UPSTREAM_ERROR"
	CodeInternal         = "INTERNAL"
	CodeNotEligible      = "NOT_ELIGIBLE"
)

// Error is an error that maps to an HTTP response. Message is safe to show users:
// it must never contain secrets, PII, SQL or upstream payloads.
type Error struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
	cause   error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return e.Code + ": " + e.Message
}

// Unwrap exposes the internal cause (logged, never returned to clients).
func (e *Error) Unwrap() error { return e.cause }

// Wrap attaches an internal cause for logging.
func (e *Error) Wrap(cause error) *Error { c := *e; c.cause = cause; return &c }

func newErr(status int, code, msg string) *Error {
	return &Error{Status: status, Code: code, Message: msg}
}

func BadRequest(msg string) *Error { return newErr(http.StatusBadRequest, CodeBadRequest, msg) }
func Unauthenticated() *Error {
	return newErr(http.StatusUnauthorized, CodeUnauthenticated, "Please sign in again.")
}
func Forbidden(msg string) *Error { return newErr(http.StatusForbidden, CodeForbidden, msg) }
func NotFound(what string) *Error {
	return newErr(http.StatusNotFound, CodeNotFound, what+" not found.")
}
func Conflict(msg string) *Error     { return newErr(http.StatusConflict, CodeConflict, msg) }
func RateLimited(msg string) *Error  { return newErr(http.StatusTooManyRequests, CodeRateLimited, msg) }
func InvalidState(msg string) *Error { return newErr(http.StatusConflict, CodeInvalidState, msg) }
func LimitReached(msg string) *Error { return newErr(http.StatusConflict, CodeLimitReached, msg) }
func Upstream(msg string) *Error     { return newErr(http.StatusBadGateway, CodeUpstream, msg) }
func Internal() *Error {
	return newErr(http.StatusInternalServerError, CodeInternal, "Something went wrong. Please try again.")
}

// Coded builds an error with an explicit status and code (e.g. CodePlanRequired, 403).
func Coded(status int, code, msg string) *Error { return newErr(status, code, msg) }

// Validation returns a 422 with per-field messages.
func Validation(fields map[string]string) *Error {
	e := newErr(http.StatusUnprocessableEntity, CodeValidation, "Some fields need attention.")
	e.Fields = fields
	return e
}

// NotEligible returns a 403 for location eligibility (reason / action from platform/eligibility).
func NotEligible(reason, action string) *Error {
	e := newErr(http.StatusForbidden, CodeNotEligible, "You are not eligible for this event.")
	e.Fields = map[string]string{"reason": reason, "action": action}
	return e
}

// As extracts an *Error from err.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
