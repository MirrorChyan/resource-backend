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
	ResourceID  string
	VersionName string
	OS          string
	Arch        string
}

type ProcessUpdateParam struct {
	ResourceID         string
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
