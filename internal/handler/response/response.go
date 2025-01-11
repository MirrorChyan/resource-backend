package response

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
	var msgStr string
	if len(msg) > 0 {
		msgStr = msg[0]
	} else {
		msgStr = "success"
	}
	return New(CodeSuccess, msgStr, data)
}

func BusinessError(msg string) *Response {
	return New(CodeBusiness, msg, nil)
}

func UnexpectedError(msg ...string) *Response {
	var msgStr string
	if len(msg) > 0 {
		msgStr = msg[0]
	} else {
		msgStr = "internal server error"
	}
	return New(CodeUnexpected, msgStr, nil)
}
