package response

const (
	CodeSuccess    = 0
	CodeBusiness   = 1
	CodeUnexpected = -1
)

type Response struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

func New(code int, msg string, data interface{}) *Response {
	return &Response{
		Code: code,
		Msg:  msg,
		Data: data,
	}
}

func Success(data interface{}) *Response {
	return New(CodeSuccess, "success", data)
}

func BusinessError(msg string) *Response {
	return New(CodeBusiness, msg, nil)
}

func UnexpectedError(msg string) *Response {
	return New(CodeUnexpected, msg, nil)
}
