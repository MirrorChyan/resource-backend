package model

import "github.com/MirrorChyan/resource-backend/internal/ent"

type UpdateResourceParam struct {
	ID          string
	Name        string
	Description string
}

type ListResourceParam struct {
	Offset int
	Limit  int
}

type CreateResourceParam struct {
	ID          string
	Name        string
	Description string
}

type CreateStorageParam struct {
	VersionID int
	Directory string
}

type CreateVersionParam struct {
	ResourceID        string
	Name              string
	UploadArchivePath string
}

type ListVersionParam struct {
	ResourceID string
	Offset     int
	Limit      int
}

type GetVersionByNameParam struct {
	ResourceID string
	Name       string
}

type VersionNameExistsParam struct {
	ResourceID string
	Name       string
}

type ValidateCDKRequest struct {
	CDK             string `json:"cdk"`
	SpecificationID string `json:"specificationId"`
	Source          string `json:"source"`
	UA              string `json:"ua"`
}

type ValidateCDKResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data bool   `json:"data"`
}

type GetLatestVersionRequest struct {
	CurrentVersion string `query:"current_version"`
	CDK            string `query:"cdk"`
	SpID           string `query:"sp_id"`
	UserAgent      string `query:"user_agent"`
}

type QueryLatestResponseData struct {
	VersionName   string `json:"version_name"`
	VersionNumber uint64 `json:"version_number"`
	Url           string `json:"url,omitempty"`
}

type CreateVersionResponseData struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Number uint64 `json:"number"`
}

type ValidateUploaderResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type CreateResourceRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type CreateResourceResponseData struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type TempDownloadInfo struct {
	ResourceID               string            `json:"resource_id"`
	Full                     bool              `json:"full"`
	TargetVersionID          int               `json:"target_version_id"`
	TargetVersionFileHashes  map[string]string `json:"target_version_file_hashes"`
	CurrentVersionID         int               `json:"current_version_id"`
	CurrentVersionFileHashes map[string]string `json:"current_version_file_hashes"`
}

type BillingCheckinRequest struct {
	CDK         string `json:"cdk"`
	Application string `json:"application"`
	Module      string `json:"module"`
	UserAgent   string `json:"user_agent"`
}

type StoreTempDownloadInfoParam struct {
	ResourceID         string
	CurrentVersionName string
	LatestVersion      *ent.Version
}

type GetResourcePathParam struct {
	ResourceID string
	VersionID  int
}

type GetVersionPatchParam struct {
	ResourceID               string
	CurrentVersionID         int
	CurrentVersionFileHashes map[string]string
	TargetVersionID          int
	TargetVersionFileHashes  map[string]string
}
