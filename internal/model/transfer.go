package model

import (
	"database/sql"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"time"
)

type LatestVersionInfo struct {
	// by logic injection
	ResourceUpdateType types.Update
	VersionId          int            `db:"version_id"`
	VersionName        string         `db:"version_name"`
	VersionNumber      uint64         `db:"version_number"`
	ReleaseNote        string         `db:"release_note"`
	CustomData         string         `db:"custom_data"`
	OS                 string         `db:"os"`
	Arch               string         `db:"arch"`
	Channel            string         `db:"channel"`
	PackageHash        sql.NullString `db:"package_hash_sha256"`
	PackagePath        sql.NullString `db:"package_path"`
	CreatedAt          time.Time      `db:"created_at"`
	VersionSerial      int            `db:"version_serial"`
}
