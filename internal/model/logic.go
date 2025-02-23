package model

import "github.com/MirrorChyan/resource-backend/internal/ent"

type CreateResourceParam struct {
	ID          string
	Name        string
	Description string
}

type CreateVersionParam struct {
	ResourceID        string
	Name              string
	OS                string
	Arch              string
	Channel           string
	UploadArchivePath string
}

type GetVersionByNameParam struct {
	ResourceID  string
	VersionName string
}

type ExistVersionNameWithOSAndArchParam struct {
	ResourceId  string
	VersionName string
	OS          string
	Arch        string
}

type ProcessUpdateParam struct {
	ResourceId         string
	CurrentVersionName string
	TargetVersion      *ent.Version
	OS                 string
	Arch               string
}

type UpdateReleaseNoteDetailParam struct {
	VersionID         int
	ReleaseNoteDetail string
}

type UpdateReleaseNoteSummaryParam struct {
	VersionID          int
	ReleaseNoteSummary string
}

type UpdateRequestParam struct {
	ResourceId         string
	CurrentVersionName string
	TargetVersionInfo  *LatestVersionInfo
}

type UpdateInfoTuple struct {
	PackageHash string
	PackagePath string
	UpdateType  string
}
type PatchTaskPayload struct {
	CurrentVersionId int
	TargetVersionId  int
	OS               string
	Arch             string
}

type IncrementalUpdateInfo struct {
	Storage *ent.Storage
}
type MultiVersionInfo struct {
	LatestVersionInfo *LatestVersionInfo
}

type PatchTaskExecuteParam struct {
	ResourceId           string
	TargetResourcePath   string
	TargetVersionId      int
	CurrentVersionId     int
	TargetStorageHashes  map[string]string
	CurrentStorageHashes map[string]string
	OS                   string
	Arch                 string
}
