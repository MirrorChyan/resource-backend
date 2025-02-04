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

type VersionNameExistsParam struct {
	ResourceID string
	Name       string
	OS         string
	Arch       string
}

type ProcessUpdateParam struct {
	ResourceID         string
	CurrentVersionName string
	TargetVersion      *ent.Version
	OS                 string
	Arch               string
}
