package model

import "github.com/MirrorChyan/resource-backend/internal/ent"

type CreateResourceParam struct {
	ID          string
	Name        string
	Description string
	UpdateType  string
}

type CreateVersionParam struct {
	ResourceID string
	Name       string
	OS         string
	Arch       string
	Channel    string
	Filename   string
}

type CreateVersionCallBackParam struct {
	ResourceID string `json:"resource_id"`
	Name       string `json:"name"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	Channel    string `json:"channel"`
	Key        string `json:"key"`
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
	ResourceId       string
	CurrentVersionId int
	TargetVersionId  int
	OS               string
	Arch             string
}

type StorageInfoCreatePayload struct {
	ResourceId  string
	Dest        string
	VersionId   int
	VersionName string

	OS         string
	Arch       string
	Channel    string
	FileHashes map[string]string
}

type IncrementalUpdateInfo struct {
	Storage *ent.Storage
}
type MultiVersionInfo struct {
	LatestVersionInfo *LatestVersionInfo
}

type PatchTaskExecuteParam struct {
	ResourceId           string
	TargetOriginPackage  string
	TargetVersionId      int
	CurrentVersionId     int
	TargetStorageHashes  map[string]string
	CurrentStorageHashes map[string]string
	OS                   string
	Arch                 string
}
