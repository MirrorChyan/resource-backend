package errs

import (
	"fmt"
	"maps"
	"net/http"
)

var (
	ErrInvalidParams           = New(BizCodeInvalidParams, http.StatusBadRequest, "invalid params", nil)
	ErrResourceNotFound        = New(BizCodeResourceNotFound, http.StatusNotFound, "resource not found", nil)
	ErrResourceInvalidOS       = New(BizCodeResourceInvalidOS, http.StatusInternalServerError, "invalid os", nil)
	ErrResourceInvalidArch     = New(BizCodeResourceInvalidArch, http.StatusInternalServerError, "invalid arch", nil)
	ErrResourceInvalidChannel  = New(BizCodeResourceInvalidChannel, http.StatusInternalServerError, "invalid channel", nil)
	ErrResourceIDAlreadyExists = New(BizCodeResourceIDAlreadyExists, http.StatusBadRequest, "resource id already exists", nil)
)

type Error struct {
	bizCode  int
	httpCode int
	message  string
	details  map[string]any
	internal error
}

func New(bizCode, httpCode int, message string, internal error) *Error {
	return &Error{
		bizCode:  bizCode,
		httpCode: httpCode,
		message:  message,
		details:  make(map[string]any),
		internal: internal,
	}
}

func (e *Error) Error() string {

	if e.internal != nil {
		return fmt.Sprintf("%s: %v", e.message, e.internal)
	}

	return e.message
}

func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && e.bizCode == t.BizCode()
}

func (e *Error) Unwrap() error {
	return e.internal
}

func (e *Error) BizCode() int {
	return e.bizCode
}

func (e *Error) HTTPCode() int {
	return e.httpCode
}

func (e *Error) Message() string {
	return e.message
}

func (e *Error) Details() map[string]any {
	return e.details
}

func (e *Error) Wrap(err error) *Error {
	return &Error{
		bizCode:  e.bizCode,
		httpCode: e.httpCode,
		message:  e.message,
		details:  e.details,
		internal: err,
	}
}

func (e *Error) WithDetails(details map[string]any) *Error {

	if details == nil {
		details = make(map[string]any)
	}

	return &Error{
		bizCode:  e.bizCode,
		httpCode: e.httpCode,
		message:  e.message,
		details:  details,
		internal: e.internal,
	}
}

func (e *Error) AddDetail(key string, value any) *Error {

	details := make(map[string]any, len(e.details)+1)

	maps.Copy(details, e.details)

	details[key] = value

	return &Error{
		bizCode:  e.bizCode,
		httpCode: e.httpCode,
		message:  e.message,
		details:  details,
		internal: e.internal,
	}
}
