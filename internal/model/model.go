package model

import "github.com/MirrorChyan/resource-backend/internal/ent"

type ValidateCDKRequest struct {
	CDK      string `json:"cdk"`
	Resource string `json:"resource"`
	UA       string `json:"ua"`
	IP       string `json:"ip"`
}

type ValidateCDKResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type ValidateUploaderResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type TempDownloadInfo struct {
	ResourceID       string `json:"resource_id"`
	Full             bool   `json:"full"`
	TargetVersionID  int    `json:"target_version_id"`
	CurrentVersionID int    `json:"current_version_id"`
	OS               string `json:"os"`
	Arch             string `json:"arch"`
}

type BillingCheckinRequest struct {
	CDK         string `json:"cdk"`
	Application string `json:"application"`
	Module      string `json:"module"`
	UserAgent   string `json:"user_agent"`
}

type GetFullUpdatePackagePathParam struct {
	ResourceID string
	VersionID  int
	OS         string
	Arch       string
}

type UpdateProcessInfo struct {
	ResourceID       string
	TargetVersionID  int
	CurrentVersionID int
	OS               string
	Arch             string
}

type ActualUpdateProcessInfo struct {
	Info    UpdateProcessInfo
	Target  *ent.Storage
	Current *ent.Storage
}

type UpdatePackage struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type UpdateInfo struct {
	RelPath    string
	SHA256     string
	UpdateType string
}

type DistributeInfo struct {
	Region   string `json:"region"`
	CDK      string `json:"cdk"`
	RelPath  string `json:"rel_path"`
	Resource string `json:"resource"`
}
