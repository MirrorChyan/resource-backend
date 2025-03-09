package response

import "strings"

const (
	CodeSuccess    = 0
	CodeBusiness   = 1
	CodeUnexpected = -1
)

type Response struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
}

func New(code int, msg string, data any) *Response {
	return &Response{
		Code: code,
		Msg:  msg,
		Data: data,
	}
}

func Success(data any, msg ...string) *Response {
	var r = "success"
	if len(msg) > 0 {
		r = strings.Join(msg, ";")
	}
	return New(CodeSuccess, r, data)
}

func BusinessError(msg string, data ...any) *Response {
	if len(data) > 0 {
		return New(CodeBusiness, msg, data[0])
	}
	return New(CodeBusiness, msg, nil)
}

func UnexpectedError(msg ...string) *Response {
	var r = "internal server error"
	if len(msg) > 0 {
		r = strings.Join(msg, ";")
	}
	return New(CodeUnexpected, r, nil)
}
