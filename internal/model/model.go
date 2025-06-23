package model

import "github.com/MirrorChyan/resource-backend/internal/model/types"

type ValidateCDKRequest struct {
	CDK      string `json:"cdk"`
	Resource string `json:"resource"`
	UA       string `json:"ua"`
	IP       string `json:"ip"`
}

type DownloadValidateCDKRequest struct {
	CDK      string `json:"cdk"`
	Resource string `json:"resource"`
	UA       string `json:"ua"`
	IP       string `json:"ip"`
	Version  string `json:"version"`
	Filesize int64  `json:"filesize"`
}

type CDKAuthResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type ValidateResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data int64  `json:"data"`
}

type ValidateUploaderResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type UpdateInfo struct {
	RelPath    string
	SHA256     string
	UpdateType string
	Filesize   int64
}

type DistributeInfo struct {
	UA       string `json:"ua,omitempty"`
	IP       string `json:"ip,omitempty"`
	CDK      string `json:"cdk"`
	Resource string `json:"resource,omitempty"`
	Version  string `json:"version,omitempty"`
	Filesize int64  `json:"filesize,omitempty"`
	RelPath  string `json:"rel_path"`
}

type FileDetectResult struct {
	Valid    bool
	FileType types.FileType
}

type PatchInfoTuple struct {
	SrcPackage  string
	DestPackage string
	FileType    string
}
