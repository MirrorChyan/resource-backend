package errs

import (
	"errors"
	"fmt"
	"net/http"
)

var (
	ErrInvalidParams = New(BizCodeInvalidParams, http.StatusBadRequest, "invalid params", nil)

	ErrResourceNotFound                 = New(BizCodeResourceNotFound, http.StatusNotFound, "resource not found", nil)
	ErrResourceInvalidOS                = New(BizCodeResourceInvalidOS, http.StatusInternalServerError, "invalid os", nil)
	ErrResourceInvalidArch              = New(BizCodeResourceInvalidArch, http.StatusInternalServerError, "invalid arch", nil)
	ErrResourceInvalidChannel           = New(BizCodeResourceInvalidChannel, http.StatusInternalServerError, "invalid channel", nil)
	ErrResourceIDAlreadyExists          = New(BizCodeResourceIDAlreadyExists, http.StatusBadRequest, "resource id already exists", nil)
	ErrResourceVersionNameConflict      = New(BizCodeResourceVersionNameConflict, http.StatusConflict, "version name under the current platform architecture already exists", nil)
	ErrResourceVersionStorageProcessing = New(BizCodeResourceVersionStorageProcessing, http.StatusConflict, "current version storage in process", nil)
	ErrResourceVersionNameUnparsable    = New(BizResourceVersionNameUnparsable, http.StatusBadRequest, "version name is not supported for parsing, please use the stable channel", nil)
)

type Error struct {
	bizCode  int
	httpCode int
	message  string
	details  any
	internal error
}

func New(bizCode, httpCode int, message string, internal error) *Error {
	return &Error{
		bizCode:  bizCode,
		httpCode: httpCode,
		message:  message,
		internal: internal,
	}
}

func NewUnexpected(msg string, errs ...error) *Error {
	var err error
	if len(errs) != 0 {
		err = errs[0]
	}
	return &Error{
		bizCode:  -1,
		message:  msg,
		httpCode: http.StatusInternalServerError,
		internal: err,
	}
}

func NewUnchecked(msg string, errs ...error) *Error {
	var err error
	if len(errs) != 0 {
		err = errs[0]
	}
	return &Error{
		bizCode:  -1,
		message:  msg,
		httpCode: http.StatusBadRequest,
		internal: err,
	}
}

func (e *Error) Error() string {

	if e.internal != nil {
		return fmt.Sprintf("%s: %v", e.message, e.internal)
	}

	return e.message
}

func (e *Error) Is(target error) bool {
	var t *Error
	ok := errors.As(target, &t)
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

func (e *Error) Details() any {
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

func (e *Error) WithDetails(details any) *Error {

	return &Error{
		bizCode:  e.bizCode,
		httpCode: e.httpCode,
		message:  e.message,
		details:  details,
		internal: e.internal,
	}
}
