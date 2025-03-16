package misc

import "github.com/MirrorChyan/resource-backend/internal/handler/response"

type EnumerableResponse struct {
	Code int
	Msg  string
}

func (r *EnumerableResponse) Ret() *response.Response {
	return response.New(r.Code, r.Msg, nil)
}

var (
	ResourceNotFound = &EnumerableResponse{Code: 8001, Msg: "resource not found"}
	InvalidOs        = &EnumerableResponse{Code: 8002, Msg: "invalid os"}
	InvalidArch      = &EnumerableResponse{Code: 8003, Msg: "invalid arch"}
	InvalidChannel   = &EnumerableResponse{Code: 8004, Msg: "invalid channel"}
)
