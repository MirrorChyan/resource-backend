package model

type UpdateResourceParam struct {
	ID          int
	Name        string
	Description string
}

type ListResourceParam struct {
	Offset int
	Limit  int
}

type CreateResourceParam struct {
	Name        string
	Description string
}

type CreateStorageParam struct {
	VersionID int
	Directory string
}

type CreateVersionParam struct {
	ResourceID        int
	Name              string
	UploadArchivePath string
}

type ListVersionParam struct {
	ResourceID int
	Offset     int
	Limit      int
}

type GetVersionByNameParam struct {
	ResourceID int
	Name       string
}

type VersionNameExistsParam struct {
	ResourceID int
	Name       string
}

type ValidateCDKRequest struct {
	CDK             string `json:"cdk"`
	SpecificationID string `json:"specificationId"`
}

type ValidateCDKResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type GetLatestVersionRequest struct {
	CurrentVersion string `query:"current_version"`
	CDK            string `query:"cdk"`
	SpId           string `query:"sp_id"`
}

type QueryLatestResponseData struct {
	VersionName string `json:"version_name"`
	Number      uint64 `json:"version_number"`
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
	Name        string `json:"name"`
	Description string `json:"description"`
}

type CreateResourceResponseData struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type TempDownloadInfo struct {
	ID             int               `json:"id"`
	Full           bool              `json:"full"`
	VersionID      int               `json:"version_id"`
	VersionName    string            `json:"version_name"`
	CurrentVersion string            `json:"current_version"`
	FileHashes     map[string]string `json:"file_hashes"`
}
